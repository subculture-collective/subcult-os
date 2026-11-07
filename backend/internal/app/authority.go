package app

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
)

// Workspace permission roles. "member" is kept as a permanent legacy alias
// for "crew" (see migrations/000010_workspace_authority.sql and
// docs/development/authority-model.md) so every existing row and every
// existing literal "owner"/"member" call site keeps working unmodified.
// New code should assign "crew" rather than "member".
const (
	roleOwner     = "owner"
	roleOrganizer = "organizer"
	roleFinance   = "finance"
	roleDoor      = "door"
	roleCrew      = "crew"
	roleMember    = "member" // legacy alias for roleCrew
)

// assignableRoles are the roles a caller may set via the member-role-change
// endpoint. "member" is deliberately excluded: it is only read, never
// written, by new code.
var assignableRoles = map[string]bool{
	roleOwner:     true,
	roleOrganizer: true,
	roleFinance:   true,
	roleDoor:      true,
	roleCrew:      true,
}

func isAssignableRole(role string) bool {
	return assignableRoles[role]
}

// Workspace permissions. These are distinct from roles: a permission is a
// capability, a role is a named bundle of capabilities. See
// docs/development/authority-model.md for the full matrix and rationale.
const (
	permOperate             = "operate"              // baseline day-to-day workspace access
	permManageMembers       = "manage_members"       // change a member's role/expiry, revoke membership
	permManageDelegations   = "manage_delegations"   // create/revoke creator delegations
	permPublish             = "publish"              // authorize a public write on behalf of a profile
	permFinance             = "finance"              // settlement and financial operations
	permDoor                = "door"                 // door check-in operations
	permManageConsent       = "manage_consent"       // create/list/withdraw channel consent grants (CONSENT-01)
	permManageAnnouncements = "manage_announcements" // create/schedule/cancel/list announcements (SIGNAL-01)
	permManageImports       = "manage_imports"       // create review-only cultural import previews (IMPORT-01)
	// connect or revoke an external announcement channel binding (#221)
	permManageChannelBindings = "manage_channel_bindings"
)

// rolePermissions is the least-privilege permission matrix. Every role
// includes permOperate: baseline read/day-to-day access is what makes a
// role a workspace member at all. Everything else is additive per role.
var rolePermissions = map[string]map[string]bool{
	roleOwner: {
		permOperate:               true,
		permManageMembers:         true,
		permManageDelegations:     true,
		permPublish:               true,
		permFinance:               true,
		permDoor:                  true,
		permManageConsent:         true,
		permManageAnnouncements:   true,
		permManageImports:         true,
		permManageChannelBindings: true,
	},
	roleOrganizer: {
		permOperate:               true,
		permManageDelegations:     true,
		permPublish:               true,
		permManageConsent:         true,
		permManageAnnouncements:   true,
		permManageImports:         true,
		permManageChannelBindings: true,
	},
	roleFinance: {
		permOperate: true,
		permFinance: true,
	},
	roleDoor: {
		permOperate: true,
		permDoor:    true,
	},
	roleCrew: {
		permOperate: true,
	},
	roleMember: { // legacy alias for roleCrew
		permOperate: true,
	},
}

func roleHasPermission(role, permission string) bool {
	perms, ok := rolePermissions[role]
	if !ok {
		return false
	}
	return perms[permission]
}

// Sentinel errors returned by authorize and authorizePublicWrite. Callers
// that only need an allow/deny decision can treat any non-nil error as
// deny; callers that need to distinguish the reason (for example to decide
// between a 403 and a 404) can use errors.Is.
var (
	// ErrMembershipDenied means the actor has no workspace membership, or
	// their membership has been removed, revoked, or has expired. Expired
	// and revoked memberships are treated as absent, not merely
	// unauthorized: they must not leak whether a role once existed.
	ErrMembershipDenied = errors.New("workspace membership is absent, revoked, or expired")
	// ErrPermissionDenied means the actor has an active membership, but
	// its role does not carry the requested permission.
	ErrPermissionDenied = errors.New("role does not carry the requested permission")
	// ErrDelegationDenied means the creator delegation required to
	// publish on behalf of a cultural profile is missing, expired, or
	// revoked.
	ErrDelegationDenied = errors.New("creator delegation is missing, expired, or revoked")
)

// activeMembership returns the caller's current workspace role, treating a
// removed, revoked, or expired membership identically to no membership at
// all. This is the single query every authority check in this package is
// built on: requireWorkspaceRole, authorize, and authorizePublicWrite all
// route through it so a revoked or expired actor is refused everywhere at
// once.
func (a *App) activeMembership(ctx context.Context, personID, workspaceID string) (string, error) {
	if a.db == nil || personID == "" || workspaceID == "" {
		return "", ErrMembershipDenied
	}
	var role string
	err := a.db.QueryRow(ctx, `
		select role
		from workspace_members
		where workspace_id = $1
		  and person_id = $2
		  and removed_at is null
		  and revoked_at is null
		  and (expires_at is null or expires_at > now())
	`, workspaceID, personID).Scan(&role)
	if err != nil {
		return "", ErrMembershipDenied
	}
	return role, nil
}

// authorize is the central permission check: it treats an absent, revoked,
// or expired membership as no membership, then checks the active role
// against the least-privilege permission matrix.
func (a *App) authorize(ctx context.Context, personID, workspaceID, permission string) error {
	role, err := a.activeMembership(ctx, personID, workspaceID)
	if err != nil {
		return err
	}
	if !roleHasPermission(role, permission) {
		return ErrPermissionDenied
	}
	return nil
}

// requireWorkspaceRole is the HTTP-facing membership check most handlers
// use. It is built directly on activeMembership so a revoked or expired
// membership is refused exactly like an absent one, while existing call
// sites that pass literal "owner"/"member" role names keep working
// unmodified.
func (a *App) requireWorkspaceRole(r *http.Request, workspaceID string, allowed ...string) (personID string, role string, ok bool) {
	personID, ok = a.requirePersonID(r)
	if !ok {
		return "", "", false
	}
	membershipRole, err := a.activeMembership(r.Context(), personID, workspaceID)
	if err != nil {
		return "", "", false
	}
	if len(allowed) == 0 {
		return personID, membershipRole, true
	}
	for _, want := range allowed {
		if membershipRole == want {
			return personID, membershipRole, true
		}
		// Legacy call sites spell "any workspace member" as the literal
		// "member" role. Every role in the matrix carries permOperate, so
		// an organizer, finance, door or crew member satisfies that
		// baseline check exactly as a legacy member does. Owner-only call
		// sites are unaffected because they never list "member".
		if want == roleMember && roleHasPermission(membershipRole, permOperate) {
			return personID, membershipRole, true
		}
	}
	return "", "", false
}

// requirePermission is a permission-based sibling of requireWorkspaceRole
// for the new endpoints in this slice, which are defined in terms of
// capabilities (manage_members, manage_delegations) rather than role
// names.
func (a *App) requirePermission(r *http.Request, workspaceID, permission string) (personID string, ok bool) {
	personID, ok = a.requirePersonID(r)
	if !ok {
		return "", false
	}
	if err := a.authorize(r.Context(), personID, workspaceID, permission); err != nil {
		return "", false
	}
	return personID, true
}

// queryRower is the subset of pgx.Tx and pgxpool.Pool that
// activeOwnerCountTx needs, so the last-owner guard can run either inside
// an in-flight transaction or directly against the pool.
type queryRower interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// activeOwnerCountTx counts workspace members with an active (not removed,
// not revoked, not expired) owner role. Every last-owner guard in this
// package — blocking removal, demotion, and revocation of the sole owner —
// is built on this single count.
func activeOwnerCountTx(ctx context.Context, q queryRower, workspaceID string) (int, error) {
	var owners int
	err := q.QueryRow(ctx, `
		select count(*)
		from workspace_members
		where workspace_id = $1
		  and role = 'owner'
		  and removed_at is null
		  and revoked_at is null
		  and (expires_at is null or expires_at > now())
	`, workspaceID).Scan(&owners)
	return owners, err
}

// authorizePublicWrite is the publication authorization hook PUB-AUTH
// (#19) calls before a queued public write leaves the future publication
// outbox. It denies when the actor's workspace membership is revoked or
// expired, when their role lacks permPublish, or when the creator
// delegation for the target profile is missing, expired, or revoked.
// Every one of those is a deny, not merely a partial success: a queued
// write must never execute on a stale authorization decision.
func (a *App) authorizePublicWrite(ctx context.Context, actorPersonID, workspaceID, profileID string) error {
	if err := a.authorize(ctx, actorPersonID, workspaceID, permPublish); err != nil {
		return err
	}
	if a.db == nil || profileID == "" {
		return ErrDelegationDenied
	}
	var delegationID string
	err := a.db.QueryRow(ctx, `
		select id
		from creator_delegations
		where workspace_id = $1
		  and cultural_profile_id = $2
		  and revoked_at is null
		  and (expires_at is null or expires_at > now())
		order by granted_at desc
		limit 1
	`, workspaceID, profileID).Scan(&delegationID)
	if err != nil {
		return ErrDelegationDenied
	}
	return nil
}
