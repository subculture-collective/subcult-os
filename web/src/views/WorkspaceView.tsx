import { useEffect, useMemo, useState } from 'react';
import type { FormEvent } from 'react';
import { api, deleteJSON, patchJSON, postJSON } from '../api';
import { ATProtoIdentityPanel } from '../components/ATProtoIdentityPanel';
import { isClosedEvent, isDraftEvent, isPublishedEvent } from '../modules/eventLifecycle/eventLifecycle';
import {
	archiveLearningLoopCopy,
	commitmentStatusLabel,
	commitmentStatusTone,
	emptyCommitmentForm,
	emptyContactForm,
	contactFormFrom,
	emptyTemplateForm,
	eventCountLabel,
	eventStatusLabel,
	eventStatusSummary,
	eventStatusSurface,
	eventStatusTone,
	getRequestedArchiveQuery,
	getRequestedWorkspaceId,
	sortCommitments,
	sortContacts,
	sortTemplates,
	staffingStatusCopy,
	templateFormFrom,
	templatePricingLabel,
	deleteTemplateState,
} from '../modules/workspace/workspaceModel';
import {
	loadDevEmailOutbox,
	loadWorkspaceArchives,
	selectWorkspace,
	loadWorkspaceOverview,
	loadWorkspaceReminders,
	loadWorkspaceTemplates,
} from '../modules/workspace/workspaceLoaders';
import type {
  CurrentUserDTO,
  CurrentWorkspaceDTO,
  CommitmentDTO,
  DevEmailOutboxMessageDTO,
  EventDTO,
  EventTemplateDTO,
  EventStatus,
  ContactDTO,
  InvitationCreatedDTO,
  ReminderEventDTO,
  WorkspaceArchiveSummaryDTO,
  WorkspaceDTO,
} from '../domain';

type ContactFormState = ReturnType<typeof emptyContactForm>;

type CommitmentFormState = ReturnType<typeof emptyCommitmentForm>;

type TemplateFormState = {
  name: string;
  title: string;
  publicDescription: string;
  locationDisplay: string;
  ticketAllocation: string;
  pricingMode: 'free' | 'fixed';
  ticketPriceDollars: string;
  privateNotes: string;
};

function signOut() {
  void postJSON('/api/auth/logout', {}).finally(() => {
    window.location.href = '/login';
  });
}

function roleLabel(role: string) {
  return role === 'owner' ? 'Owner' : 'Member';
}

function roleHint(role: string) {
  return role === 'owner' ? 'Can invite members and publish events' : 'Can help run the room';
}

function formatDateTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat([], { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function toRfc3339DateTime(value: string) {
  if (!value) {
    return undefined;
  }

  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? undefined : date.toISOString();
}

function formatShortDateTime(value: string) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat([], { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }).format(date);
}

function parseTagList(value: string) {
  const seen = new Set<string>();
  const tags: string[] = [];

  for (const tag of value.split(',')) {
    const trimmed = tag.trim();
    if (!trimmed) continue;
    const normalized = trimmed.toLowerCase();
    if (seen.has(normalized)) continue;
    seen.add(normalized);
    tags.push(trimmed);
  }

  return tags;
}

type OperatorAction = {
  label: string;
  href: string;
  variant: 'primary' | 'secondary' | 'ghost';
};

type OperatorGuidance = {
  eyebrow: string;
  title: string;
  body: string;
  tone: 'amber' | 'emerald' | 'fuchsia' | 'zinc';
  actions: OperatorAction[];
};

function buildOperatorGuidance(events: EventDTO[], workspaceId: string): OperatorGuidance {
  const newestDraft = events.find((event) => isDraftEvent(event.status)) ?? null;
  const newestPublished = events.find((event) => isPublishedEvent(event.status)) ?? null;
  const newestClosed = events.find((event) => isClosedEvent(event.status)) ?? null;

  if (events.length === 0) {
    return {
      eyebrow: 'Start here',
      title: 'Create the first event',
      body: 'Shape the room, publish the page, and get the door ready for the first wave of guests.',
      tone: 'amber',
      actions: [
        { label: 'Create event', href: `/events/new?workspaceId=${workspaceId}`, variant: 'primary' },
        { label: 'Import occurrences', href: `/workspace/${workspaceId}/cultural-imports`, variant: 'ghost' },
        { label: 'Invite member', href: '#invite-member', variant: 'secondary' },
      ],
    };
  }

  if (newestDraft) {
    return {
      eyebrow: 'Draft ready',
      title: `Finish ${newestDraft.title}`,
      body: 'Polish the checklist, then publish when the page feels right.',
      tone: 'amber',
      actions: [
        { label: 'Continue editing', href: `/events/${newestDraft.id}`, variant: 'primary' },
        { label: 'Create next event', href: `/events/new?workspaceId=${workspaceId}`, variant: 'secondary' },
      ],
    };
  }

  if (newestPublished) {
    return {
      eyebrow: 'Live now',
      title: `${newestPublished.title} is on the floor`,
      body: 'Open the Door, share the public page, and end the night when the room quiets down.',
      tone: 'emerald',
      actions: [
        { label: 'Open Door', href: `/door/${newestPublished.id}`, variant: 'primary' },
        { label: 'Share public page', href: newestPublished.publicUrl ?? `/e/${newestPublished.id}`, variant: 'secondary' },
        { label: 'End night', href: `/events/${newestPublished.id}`, variant: 'ghost' },
      ],
    };
  }

  if (newestClosed) {
    return {
      eyebrow: 'Wrapped',
      title: `Review ${newestClosed.title}`,
      body: 'Read the report, reset the room, and set up the next event slice.',
      tone: 'fuchsia',
      actions: [
        { label: 'View report', href: `/events/${newestClosed.id}`, variant: 'primary' },
        { label: 'Create next event', href: `/events/new?workspaceId=${workspaceId}`, variant: 'secondary' },
      ],
    };
  }

  return {
    eyebrow: 'Ready',
    title: 'Run the room from here',
    body: 'Use the workspace to keep the door moving: publish the next event, invite help, and close out cleanly.',
    tone: 'zinc',
    actions: [{ label: 'Create event', href: `/events/new?workspaceId=${workspaceId}`, variant: 'primary' }],
  };
}

function toneSurface(tone: OperatorGuidance['tone']) {
  switch (tone) {
    case 'amber':
      return 'border-amber-400/20 bg-amber-400/[0.07]';
    case 'emerald':
      return 'border-emerald-400/20 bg-emerald-400/[0.07]';
    case 'fuchsia':
      return 'border-fuchsia-400/20 bg-fuchsia-400/[0.07]';
    case 'zinc':
      return 'border-white/10 bg-white/[0.04]';
  }
}

function toneLabel(tone: OperatorGuidance['tone']) {
  switch (tone) {
    case 'amber':
      return 'text-amber-200';
    case 'emerald':
      return 'text-emerald-200';
    case 'fuchsia':
      return 'text-fuchsia-200';
    case 'zinc':
      return 'text-zinc-200';
  }
}

function extractInviteToken(body: string) {
  return body.match(/\/invite\/([A-Za-z0-9_-]+)/)?.[1] ?? null;
}

function extractTicketCode(body: string) {
  return body.match(/\/tickets\/([A-Za-z0-9_-]+)/)?.[1] ?? null;
}

export function WorkspaceView() {
  const [me, setMe] = useState<CurrentUserDTO | null>(null);
  const [workspace, setWorkspace] = useState<CurrentWorkspaceDTO | null>(null);
  const [events, setEvents] = useState<EventDTO[]>([]);
  const [archives, setArchives] = useState<WorkspaceArchiveSummaryDTO[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [workspaceNotice, setWorkspaceNotice] = useState<string | null>(null);
  const [workspaceName, setWorkspaceName] = useState('');
  const [creatingWorkspace, setCreatingWorkspace] = useState(false);
  const [inviteEmail, setInviteEmail] = useState('');
  const [sendingInvite, setSendingInvite] = useState(false);
  const [inviteNotice, setInviteNotice] = useState<string | null>(null);
  const [emailOutbox, setEmailOutbox] = useState<DevEmailOutboxMessageDTO[] | null>(null);
  const [seedingEventId, setSeedingEventId] = useState<string | null>(null);
  const initialArchiveQuery = useMemo(() => getRequestedArchiveQuery().trim(), []);
  const [archiveQuery, setArchiveQuery] = useState(initialArchiveQuery);
  const [archiveSearching, setArchiveSearching] = useState(false);
  const [contacts, setContacts] = useState<ContactDTO[] | null>(null);
  const [contactsDenied, setContactsDenied] = useState(false);
  const [contactForm, setContactForm] = useState<ContactFormState>(emptyContactForm());
  const [editingContactId, setEditingContactId] = useState<string | null>(null);
  const [commitments, setCommitments] = useState<CommitmentDTO[] | null>(null);
  const [commitmentsDenied, setCommitmentsDenied] = useState(false);
  const [commitmentForm, setCommitmentForm] = useState<CommitmentFormState>(emptyCommitmentForm());
  const [templates, setTemplates] = useState<EventTemplateDTO[] | null>(null);
  const [templateForm, setTemplateForm] = useState<TemplateFormState>(emptyTemplateForm());
  const [editingTemplateId, setEditingTemplateId] = useState<string | null>(null);
  const [templateSubmitting, setTemplateSubmitting] = useState(false);
  const [templateDeletingId, setTemplateDeletingId] = useState<string | null>(null);
  const [templateNotice, setTemplateNotice] = useState<string | null>(null);
  const [reminders, setReminders] = useState<ReminderEventDTO[] | null | undefined>(undefined);
  const [reminderSweepRunning, setReminderSweepRunning] = useState(false);
  const [remindersRefreshTick, setRemindersRefreshTick] = useState(0);
  const requestedWorkspaceId = useMemo(() => getRequestedWorkspaceId(), []);

  function resetPrivateWorkspaceState() {
    setContacts(null);
    setContactsDenied(false);
    setContactForm(emptyContactForm());
    setEditingContactId(null);
    setCommitments(null);
    setCommitmentsDenied(false);
    setCommitmentForm(emptyCommitmentForm());
    setTemplates(null);
    setTemplateForm(emptyTemplateForm());
    setEditingTemplateId(null);
    setTemplateSubmitting(false);
    setTemplateDeletingId(null);
    setTemplateNotice(null);
    setReminders(undefined);
    setReminderSweepRunning(false);
  }

  const workspaceSummaries = useMemo(() => me?.workspaces ?? [], [me]);
  const orderedEvents = useMemo(() => [...events].sort((left, right) => new Date(right.startsAt).getTime() - new Date(left.startsAt).getTime()), [events]);
  const orderedArchives = useMemo(
    () => [...archives].sort((left, right) => new Date(right.startsAt).getTime() - new Date(left.startsAt).getTime() || new Date(right.createdAt).getTime() - new Date(left.createdAt).getTime()),
    [archives],
  );
  const normalizedArchiveQuery = archiveQuery.trim();
  const archiveByEventId = useMemo(() => new Map(archives.map((archive) => [archive.eventId, archive] as const)), [archives]);
  const eventTitleById = useMemo(() => new Map(orderedEvents.map((event) => [event.id, event.title] as const)), [orderedEvents]);
  const archiveLearningLoop = useMemo(
    () => (normalizedArchiveQuery ? 'Search results are filtered. Reset to see the full workspace learning loop.' : archiveLearningLoopCopy(orderedArchives)),
    [normalizedArchiveQuery, orderedArchives],
  );
  const visibleContacts = useMemo(() => (contacts ? sortContacts(contacts) : []), [contacts]);
  const visibleCommitments = useMemo(() => (commitments ? sortCommitments(commitments) : []), [commitments]);
  const visibleReminders = useMemo(() => (Array.isArray(reminders) ? reminders : []), [reminders]);
  const commitmentCounts = useMemo(
    () =>
      visibleCommitments.reduce(
        (counts, commitment) => ({
          ...counts,
          [commitment.status]: counts[commitment.status] + 1,
        }),
        { open: 0, done: 0, cancelled: 0 },
      ),
    [visibleCommitments],
  );
  const statusCounts = useMemo(
    () =>
      orderedEvents.reduce(
        (accumulator, event) => {
          accumulator[event.status] += 1;
          return accumulator;
        },
        { draft: 0, published: 0, end_of_night: 0 } satisfies Record<EventStatus, number>,
      ),
    [orderedEvents],
  );
  const guidance = useMemo(() => buildOperatorGuidance(orderedEvents, workspace?.id ?? ''), [orderedEvents, workspace?.id]);

  useEffect(() => {
    let cancelled = false;

    async function loadWorkspaceData(nextWorkspace: CurrentWorkspaceDTO) {
      const {events: loadedEvents, archives: loadedArchives, contacts: loadedContacts, commitments: loadedCommitments} = await loadWorkspaceOverview(nextWorkspace.id, initialArchiveQuery);
      if (!cancelled) {
        resetPrivateWorkspaceState();
        setWorkspace(nextWorkspace);
        setEvents(loadedEvents ?? []);
        setArchives(loadedArchives ?? []);
        setContactsDenied(loadedContacts.denied);
        setContacts(loadedContacts.data);
        setCommitmentsDenied(loadedCommitments.denied);
        setCommitments(loadedCommitments.data);
      }
    }

    async function load() {
      setLoading(true);
      setError(null);
      setWorkspaceNotice(null);

      try {
        const user = await api<CurrentUserDTO>('/api/me');
        if (cancelled) return;
        setMe(user);

        if (user.workspaces.length === 0) {
          setWorkspace(null);
          setEvents([]);
          setArchives([]);
          return;
        }

        const fallback = await selectWorkspace(user, requestedWorkspaceId);
        if (cancelled) return;

        if (fallback) {
          await loadWorkspaceData(fallback.workspace);
          if (!cancelled) setWorkspaceNotice(fallback.notice);
        } else {
          setWorkspace(null);
          setEvents([]);
          setArchives([]);
        }
      } catch (caught) {
        if (!cancelled) {
          setError(caught instanceof Error ? caught.message : 'Unable to load workspace');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }

    void load();

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;

    async function loadReminders() {
      if (!workspace) {
        setReminders(undefined);
        return;
      }

      setReminders(undefined);

      try {
        const loadedReminders = await loadWorkspaceReminders(workspace.id);
        if (!cancelled) {
          setReminders(loadedReminders);
        }
      } catch (caught) {
        if (!cancelled) {
          setError(caught instanceof Error ? caught.message : 'Unable to load reminder activity');
          setReminders(null);
        }
      }
    }

    void loadReminders();

    return () => {
      cancelled = true;
    };
  }, [remindersRefreshTick, workspace?.id]);

  useEffect(() => {
    let cancelled = false;

    async function loadOutbox() {
      if (!workspace) {
        setEmailOutbox(null);
        return;
      }

      const loaded = await loadDevEmailOutbox();
      if (!cancelled) {
        setEmailOutbox(loaded ? [...loaded].sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()).slice(0, 5) : null);
      }
    }

    void loadOutbox();

    return () => {
      cancelled = true;
    };
  }, [workspace?.id]);

  useEffect(() => {
    let cancelled = false;

    async function loadTemplates() {
      if (!workspace) {
        setTemplates(null);
        setTemplateForm(emptyTemplateForm());
        setEditingTemplateId(null);
        setTemplateSubmitting(false);
        setTemplateDeletingId(null);
        setTemplateNotice(null);
        return;
      }

      try {
        const loadedTemplates = await loadWorkspaceTemplates(workspace.id);
        if (!cancelled) {
          setTemplates(loadedTemplates ? sortTemplates(loadedTemplates) : null);
        }
      } catch (caught) {
        if (!cancelled) {
          setTemplates(null);
          setError(caught instanceof Error ? caught.message : 'Unable to load event templates');
        }
      }
    }

    void loadTemplates();

    return () => {
      cancelled = true;
    };
  }, [workspace?.id]);

  async function handleCreateWorkspace(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const name = workspaceName.trim();
    if (!name) {
      setError('Enter a Workspace name before creating it.');
      return;
    }

    setCreatingWorkspace(true);
    setError(null);

    try {
      const created = await postJSON<WorkspaceDTO>('/api/workspaces', { name });
      const nextWorkspace: CurrentWorkspaceDTO = {
        ...created,
        members: [],
        invitations: [],
      };
      resetPrivateWorkspaceState();
      setWorkspace(nextWorkspace);
      setEvents([]);
      setArchives([]);
      setWorkspaceName('');
      setWorkspaceNotice(`Created ${created.name}. You can invite members or start the first event now.`);
      setMe((current) =>
        current
          ? {
              ...current,
              workspaces: [created, ...current.workspaces.filter((summary) => summary.id !== created.id)],
            }
          : current,
      );
      if (typeof window !== 'undefined') {
        window.history.pushState({}, '', `/workspace?workspaceId=${created.id}`);
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to create workspace');
    } finally {
      setCreatingWorkspace(false);
    }
  }

  async function handleInvite(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!workspace) return;

    const email = inviteEmail.trim();
    if (!email) {
      setError('Enter an email address before sending an invite.');
      return;
    }

    setSendingInvite(true);
    setError(null);
    setInviteNotice(null);

    try {
      const created = await postJSON<InvitationCreatedDTO>(`/api/workspaces/${workspace.id}/invitations`, { email });
      setInviteEmail('');
      setWorkspace((current) =>
        current
          ? {
              ...current,
              invitations: [
                ...current.invitations,
                {
                  id: created.id,
                  email: created.email,
                  role: created.role,
                  token: created.token,
                  acceptedAt: null,
                },
              ],
            }
          : current,
      );
      setInviteNotice(`Invite sent to ${created.email}.`);
      const loaded = await loadDevEmailOutbox();
      setEmailOutbox(loaded ? [...loaded].sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()).slice(0, 5) : null);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to send invite');
    } finally {
      setSendingInvite(false);
    }
  }

  async function handleSeedNextDraft(eventID: string) {
    if (!workspace || workspace.role !== 'owner') {
      return;
    }

    setError(null);
    setSeedingEventId(eventID);

    try {
      const seeded = await postJSON<EventDTO>(`/api/events/${eventID}/archive/seed-draft`, {});
      setEvents((current) => [...current.filter((event) => event.id !== seeded.id), seeded]);
      setArchives((current) => current.map((archive) => (archive.eventId === eventID ? { ...archive, seededEventId: seeded.id } : archive)));
      setWorkspaceNotice(`Seeded next draft: ${seeded.title}`);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to seed next draft');
    } finally {
      setSeedingEventId((current) => (current === eventID ? null : current));
    }
  }

  async function handleArchiveSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!workspace) {
      return;
    }

    const nextQuery = archiveQuery.trim();
    setArchiveQuery(nextQuery);
    setArchiveSearching(true);
    setError(null);

    try {
      const loaded = await loadWorkspaceArchives(workspace.id, nextQuery);
      setArchives(loaded ?? []);
      setError(null);
      if (typeof window !== 'undefined') {
        const suffix = nextQuery ? `&q=${encodeURIComponent(nextQuery)}` : '';
        window.history.pushState({}, '', `/workspace?workspaceId=${workspace.id}${suffix}`);
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to search archives');
    } finally {
      setArchiveSearching(false);
    }
  }

  async function handleArchiveReset() {
    if (!workspace) {
      return;
    }

    setArchiveQuery('');
    setArchiveSearching(true);
    setError(null);

    try {
      const loaded = await loadWorkspaceArchives(workspace.id, '');
      setArchives(loaded ?? []);
      setError(null);
      if (typeof window !== 'undefined') {
        window.history.pushState({}, '', `/workspace?workspaceId=${workspace.id}`);
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to reset archive search');
    } finally {
      setArchiveSearching(false);
    }
  }

  function resetContactEditor() {
    setEditingContactId(null);
    setContactForm(emptyContactForm());
  }

  function editContact(contact: ContactDTO) {
    setEditingContactId(contact.id);
    setContactForm(contactFormFrom(contact));
  }

  async function handleContactSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!workspace || workspace.role !== 'owner') {
      return;
    }

    const displayName = contactForm.displayName.trim();
    if (!displayName) {
      setError('Enter a contact name before saving it.');
      return;
    }

    const email = contactForm.email.trim();
    const phone = contactForm.phone.trim();
    const notes = contactForm.notes.trim();
    const tags = parseTagList(contactForm.tags);

    setError(null);

    try {
      const payload = editingContactId
        ? {
            displayName,
            email: email || undefined,
            phone: phone || undefined,
            notes,
            tags,
            clearEmail: !email,
            clearPhone: !phone,
          }
        : {
            displayName,
            email: email || undefined,
            phone: phone || undefined,
            notes,
            tags,
          };

      if (editingContactId) {
        const updated = await patchJSON<ContactDTO>(`/api/workspaces/${workspace.id}/contacts/${editingContactId}`, payload);
        setContacts((current) => sortContacts([...(current ?? []).filter((contact) => contact.id !== updated.id), updated]));
      } else {
        const created = await postJSON<ContactDTO>(`/api/workspaces/${workspace.id}/contacts`, payload);
        setContacts((current) => sortContacts([...(current ?? []), created]));
      }

      resetContactEditor();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to save contact');
    }
  }

  async function handleCommitmentSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!workspace || workspace.role !== 'owner') {
      return;
    }

    const title = commitmentForm.title.trim();
    if (!title) {
      setError('Enter a commitment title before saving it.');
      return;
    }

    setError(null);

    try {
      const payload = {
        title,
        description: commitmentForm.description.trim(),
        dueAt: toRfc3339DateTime(commitmentForm.dueAt),
        eventId: commitmentForm.eventId || undefined,
      };

      const created = await postJSON<CommitmentDTO>(`/api/workspaces/${workspace.id}/commitments`, payload);
      setCommitments((current) => sortCommitments([...(current ?? []), created]));
      setCommitmentForm(emptyCommitmentForm());
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to save commitment');
    }
  }

  async function handleCommitmentStatus(commitmentID: string, status: CommitmentDTO['status']) {
    if (!workspace || workspace.role !== 'owner') {
      return;
    }

    setError(null);

    try {
      const updated = await patchJSON<CommitmentDTO>(`/api/workspaces/${workspace.id}/commitments/${commitmentID}`, { status });
      setCommitments((current) => sortCommitments([...(current ?? []).filter((commitment) => commitment.id !== updated.id), updated]));
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to update commitment');
    }
  }

  async function handleRunReminderSweep() {
    if (!workspace || workspace.role !== 'owner') {
      return;
    }

    setReminderSweepRunning(true);
    setError(null);

    try {
      await postJSON<void>(`/api/workspaces/${workspace.id}/reminders/sweep`, {});
      setReminders(undefined);
      setRemindersRefreshTick((tick) => tick + 1);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to sweep reminders');
    } finally {
      setReminderSweepRunning(false);
    }
  }

  function resetTemplateEditor() {
    setEditingTemplateId(null);
    setTemplateForm(emptyTemplateForm());
  }

  function editTemplate(template: EventTemplateDTO) {
    setEditingTemplateId(template.id);
    setTemplateForm(templateFormFrom(template));
    setTemplateNotice(null);
  }

  async function handleTemplateSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!workspace || workspace.role !== 'owner') {
      return;
    }

    const name = templateForm.name.trim();
    const title = templateForm.title.trim();
    if (!name || !title) {
      setError('Enter a template name and title before saving it.');
      return;
    }

    const publicDescription = templateForm.publicDescription.trim();
    const locationDisplay = templateForm.locationDisplay.trim();
    const privateNotes = templateForm.privateNotes.trim();
    const ticketAllocation = Number(templateForm.ticketAllocation);
    if (!Number.isInteger(ticketAllocation) || ticketAllocation < 0) {
      setError('Ticket allocation must be zero or greater.');
      return;
    }

    const ticketPriceCents = templateForm.pricingMode === 'fixed' ? Math.round(Number(templateForm.ticketPriceDollars) * 100) : 0;
    if (templateForm.pricingMode === 'fixed' && ticketPriceCents < 50) {
      setError('Fixed templates need a ticket price of at least $0.50.');
      return;
    }

    setTemplateSubmitting(true);
    setError(null);
    setTemplateNotice(null);

    const payload = {
      name,
      title,
      publicDescription,
      locationDisplay,
      ticketAllocation,
      pricingMode: templateForm.pricingMode,
      ticketPriceCents,
      ticketCurrency: 'usd',
      privateNotes,
    };

    try {
      if (editingTemplateId) {
        const updated = await patchJSON<EventTemplateDTO>(`/api/workspaces/${workspace.id}/event-templates/${editingTemplateId}`, payload);
        setTemplates((current) => sortTemplates([...(current ?? []).filter((template) => template.id !== updated.id), updated]));
        setTemplateNotice(`Updated ${updated.name}.`);
      } else {
        const created = await postJSON<EventTemplateDTO>(`/api/workspaces/${workspace.id}/event-templates`, payload);
        setTemplates((current) => sortTemplates([...(current ?? []), created]));
        setTemplateNotice(`Created ${created.name}.`);
      }

      resetTemplateEditor();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to save template');
    } finally {
      setTemplateSubmitting(false);
    }
  }

  async function handleTemplateDelete(templateID: string) {
    if (!workspace || workspace.role !== 'owner') {
      return;
    }

    setTemplateDeletingId(templateID);
    setError(null);
    setTemplateNotice(null);

    try {
      await deleteJSON(`/api/workspaces/${workspace.id}/event-templates/${templateID}`);
      setTemplates((current) => deleteTemplateState({ templates: current, editingTemplateId, templateDeletingId }, templateID).templates);
      if (editingTemplateId === templateID) {
        resetTemplateEditor();
      }
      setTemplateNotice('Template deleted.');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to delete template');
    } finally {
      setTemplateDeletingId((current) => (current === templateID ? null : current));
    }
  }

  const workspaceId = workspace?.id ?? '';

  return (
    <main className="min-h-screen px-4 py-6 text-zinc-100 sm:px-6 lg:px-8">
      <section className="mx-auto w-full max-w-6xl space-y-6">
        <header className="flex flex-col gap-4 rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6 shadow-2xl shadow-black/40 backdrop-blur sm:flex-row sm:items-center sm:justify-between">
          <div>
            <p className="text-xs uppercase tracking-[0.3em] text-fuchsia-300">subcult-os</p>
            <h1 className="mt-2 text-3xl font-semibold tracking-tight text-white">Operator home</h1>
            <p className="mt-2 max-w-2xl text-sm leading-6 text-zinc-400">
              Run the room from one place: create the next event, invite help, and keep the Door moving.
            </p>
          </div>

          <div className="flex flex-wrap gap-2 text-sm">
			{me ? <a className="rounded-full border border-white/10 bg-white/5 px-4 py-2 text-zinc-200 transition hover:bg-white/10" href="/participant">
			  My assignments
			</a> : null}
            <a className="rounded-full border border-white/10 bg-white/5 px-4 py-2 text-zinc-200 transition hover:bg-white/10" href="/login">
              Auth
            </a>
            <a className="rounded-full border border-white/10 bg-white/5 px-4 py-2 text-zinc-200 transition hover:bg-white/10" href="/discover">
              Public discovery
            </a>
            <button className="rounded-full border border-white/10 bg-white/5 px-4 py-2 text-zinc-200 transition hover:bg-white/10" type="button" onClick={signOut}>
              Sign out
            </button>
          </div>
        </header>

        {error ? <div role="alert" className="rounded-2xl border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
          <p>{error}</p>
          {!workspace && !loading ? <button type="button" className="mt-3 rounded-full border border-white/20 px-4 py-2" onClick={() => window.location.reload()}>Retry workspace</button> : null}
        </div> : null}
        {workspaceNotice ? <p className="rounded-2xl border border-amber-400/20 bg-amber-400/10 px-4 py-3 text-sm text-amber-100">{workspaceNotice}</p> : null}
        {!loading && me ? <ATProtoIdentityPanel /> : null}

        {loading ? (
          <section className="grid gap-6 lg:grid-cols-[1.15fr_0.85fr]">
            <div className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6 shadow-xl shadow-black/30">
              <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Loading workspace</p>
              <h2 className="mt-2 text-3xl font-semibold tracking-tight text-white">Finding the right room</h2>
              <p className="mt-2 max-w-2xl text-sm leading-6 text-zinc-400">We are checking your current Workspace, loading its events, and preparing the operator dashboard.</p>
            </div>

            <aside className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
              <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Workspace access</p>
              <p className="mt-2 text-sm leading-6 text-zinc-400">One person can operate multiple Workspaces. Use this switcher to jump between them.</p>
              <a className="mt-4 inline-flex rounded-full border border-white/10 bg-white/5 px-4 py-2 text-sm text-zinc-200 transition hover:bg-white/10" href="/discover">
                Public discovery
              </a>
              <div className="mt-4 space-y-2">
                <div className="rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-sm text-zinc-500">Loading access…</div>
              </div>
            </aside>
          </section>
        ) : me && me.workspaces.length === 0 ? (
          <section className="grid gap-6 lg:grid-cols-[1.2fr_0.8fr]">
            <div className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
              <p className="text-xs uppercase tracking-[0.3em] text-amber-300">No workspace yet</p>
              <h2 className="mt-2 text-2xl font-semibold text-white">Create one to start</h2>
              <p className="mt-2 text-sm leading-6 text-zinc-400">You need a workspace before you can invite members or publish events.</p>
            </div>

            <form className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6" onSubmit={handleCreateWorkspace}>
              <label className="block space-y-2 text-sm">
                <span className="text-zinc-300">Workspace name</span>
                <input
                  className="w-full rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-white/8"
                  value={workspaceName}
                  onChange={(event) => setWorkspaceName(event.target.value)}
                  required
                />
              </label>
              <button className="mt-4 w-full rounded-2xl bg-amber-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-amber-200 disabled:cursor-not-allowed disabled:bg-amber-300/60" type="submit" disabled={creatingWorkspace}>
                {creatingWorkspace ? 'Creating…' : 'Create workspace'}
              </button>
            </form>
          </section>
        ) : workspace ? (
          <>
            <section className="grid gap-6 lg:grid-cols-[1.15fr_0.85fr]">
              <div className="space-y-6">
                <div className="grid gap-4 xl:grid-cols-[1.08fr_0.92fr]">
                  <div className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6 shadow-xl shadow-black/30">
                    <div className="flex flex-wrap items-start justify-between gap-3">
                      <div>
                        <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Current workspace</p>
                        <h2 className="mt-2 text-3xl font-semibold tracking-tight text-white">{workspace.name}</h2>
                        <p className="mt-2 text-sm text-zinc-400">You are signed in as {me?.email ?? 'a member'}.</p>
                      </div>
                      <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs uppercase tracking-[0.25em] text-zinc-300">{roleLabel(workspace.role)}</span>
                    </div>

                    <div className="mt-6 grid gap-3 sm:grid-cols-3">
                      <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Members</p>
                        <p className="mt-2 text-2xl font-semibold text-white">{workspace.members.length}</p>
                      </div>
                      <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Invites</p>
                        <p className="mt-2 text-2xl font-semibold text-white">{workspace.invitations.length}</p>
                      </div>
                      <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Events</p>
                        <p className="mt-2 text-2xl font-semibold text-white">{events.length}</p>
                      </div>
                    </div>

                    <div className="mt-6 grid gap-3 sm:grid-cols-3">
                      <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Draft</p>
                        <p className="mt-2 text-2xl font-semibold text-white">{statusCounts.draft}</p>
                      </div>
                      <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Live</p>
                        <p className="mt-2 text-2xl font-semibold text-white">{statusCounts.published}</p>
                      </div>
                      <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Closed</p>
                        <p className="mt-2 text-2xl font-semibold text-white">{statusCounts.end_of_night}</p>
                      </div>
                    </div>
                  </div>

                  <div className={`rounded-[1.75rem] border p-6 shadow-xl shadow-black/20 ${toneSurface(guidance.tone)}`}>
                    <p className={`text-xs uppercase tracking-[0.3em] ${toneLabel(guidance.tone)}`}>{guidance.eyebrow}</p>
                    <h3 className="mt-2 text-2xl font-semibold tracking-tight text-white">{guidance.title}</h3>
                    <p className="mt-3 max-w-xl text-sm leading-6 text-zinc-300">{guidance.body}</p>
                    <div className="mt-6 flex flex-wrap gap-2 text-sm">
                      {guidance.actions.map((action) =>
                        action.variant === 'primary' ? (
                          <a key={action.label} className="rounded-full bg-amber-300 px-4 py-2 font-medium text-zinc-950 transition hover:bg-amber-200" href={action.href}>
                            {action.label}
                          </a>
                        ) : action.variant === 'secondary' ? (
                          <a key={action.label} className="rounded-full border border-white/10 bg-white/5 px-4 py-2 text-zinc-200 transition hover:bg-white/10" href={action.href}>
                            {action.label}
                          </a>
                        ) : (
                          <a key={action.label} className="rounded-full border border-white/10 bg-black/15 px-4 py-2 text-zinc-200 transition hover:bg-black/30" href={action.href}>
                            {action.label}
                          </a>
                        ),
                      )}
                    </div>
                  </div>
                </div>

                <div className="mt-6 space-y-3">
                  <p className="text-xs uppercase tracking-[0.3em] text-zinc-500">Members</p>
                  <div className="space-y-2">
                    {workspace.members.length === 0 ? (
                      <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-5 text-sm text-zinc-400">
                        <p className="font-medium text-white">No members yet</p>
                        <p className="mt-1 leading-6">Invite the first operator and this roster will populate automatically.</p>
                      </div>
                    ) : null}
                    {workspace.members.map((member) => (
                      <div key={member.id} className="flex items-center justify-between gap-3 rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-sm">
                        <div>
                          <p className="font-medium text-white">{member.displayName ?? member.email}</p>
                          <p className="text-zinc-400">{member.email}</p>
                        </div>
                        <div className="text-right">
                          <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1 text-[11px] uppercase tracking-[0.25em] text-zinc-300">{roleLabel(member.role)}</span>
                          <p className="mt-2 text-xs leading-5 text-zinc-500">{roleHint(member.role)}</p>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>

                <div className="mt-6 space-y-3">
                  <div className="flex items-center justify-between gap-3">
                    <p className="text-xs uppercase tracking-[0.3em] text-zinc-500">Events</p>
                    <a className="rounded-full bg-amber-300 px-3 py-2 text-xs font-medium uppercase tracking-[0.25em] text-zinc-950 transition hover:bg-amber-200" href={`/events/new?workspaceId=${workspace.id}`}>
                      New event
                    </a>
                  </div>

                  <div className="space-y-3">
                    {events.length === 0 ? (
                      <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-5 text-sm text-zinc-400">
                        <p className="font-medium text-white">No events yet</p>
                        <p className="mt-1 leading-6">Create the first event to turn this workspace into a live operator home.</p>
                      </div>
                    ) : null}
                    {orderedEvents.map((event) => {
                      const archive = archiveByEventId.get(event.id) ?? null;

                      return (
                      <article key={event.id} className={`rounded-[1.5rem] border p-4 ${eventStatusSurface(event.status)}`}>
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div>
                            <h3 className="text-lg font-medium text-white">{event.title}</h3>
                            <p className="mt-1 text-sm text-zinc-400">{formatDateTime(event.startsAt)}</p>
                            <p className="mt-2 text-sm leading-6 text-zinc-300 line-clamp-3">{event.publicDescription}</p>
                          </div>
                          <span className={`rounded-full border px-3 py-1 text-xs uppercase tracking-[0.25em] ${eventStatusTone(event.status)}`}>{eventStatusLabel(event.status)}</span>
                        </div>
                        <p className="mt-3 text-sm font-medium text-zinc-200">{eventStatusSummary(event.status)}</p>
                        <div className="mt-4 rounded-2xl border border-white/10 bg-black/20 px-4 py-3 text-sm text-zinc-300">{eventCountLabel(event)}</div>
                        {!isDraftEvent(event.status) ? <p className="mt-3 text-sm leading-6 text-zinc-400">{staffingStatusCopy(event)}</p> : null}
                        <div className="mt-4 flex flex-wrap gap-2 text-sm">
                          <a className="rounded-full bg-white px-3 py-2 font-medium text-zinc-950 transition hover:bg-zinc-200" href={`/events/${event.id}`}>
                            {isDraftEvent(event.status) ? 'Finish draft' : isPublishedEvent(event.status) ? 'View editor' : 'Open archive'}
                          </a>
                          {isDraftEvent(event.status) ? (
                            <a className="rounded-full border border-white/10 bg-white/5 px-3 py-2 text-zinc-200 transition hover:bg-white/10" href={`/events/${event.id}`}>
                              Publish checklist
                            </a>
                          ) : null}
                          {isPublishedEvent(event.status) ? (
                            <>
                              <a className="rounded-full border border-white/10 bg-white/5 px-3 py-2 text-zinc-200 transition hover:bg-white/10" href={`/door/${event.id}`}>
                                Open Door
                              </a>
                              {event.publicUrl ? (
                                <a className="rounded-full border border-white/10 bg-white/5 px-3 py-2 text-zinc-200 transition hover:bg-white/10" href={event.publicUrl}>
                                  Share public page
                                </a>
                              ) : null}
                              <a className="rounded-full border border-white/10 bg-black/15 px-3 py-2 text-zinc-200 transition hover:bg-black/30" href={`/events/${event.id}`}>
                                End night
                              </a>
                            </>
                          ) : null}
                          {isClosedEvent(event.status) ? (
                            <>
                              <div className="rounded-2xl border border-fuchsia-400/20 bg-fuchsia-400/10 px-3 py-3 text-sm text-fuchsia-50">
                                <p className="text-[11px] uppercase tracking-[0.25em] text-fuchsia-100">Archive ready after closeout</p>
                                <p className="mt-2 leading-6">Use the private archive to seed the next draft from the event editor.</p>
                              </div>
                              {workspace?.role === 'owner' ? (
                                archive?.seededEventId ? (
                                  <a className="rounded-full border border-violet-400/20 bg-violet-300 px-3 py-2 text-zinc-950 transition hover:bg-violet-200" href={`/events/${archive.seededEventId}`}>
                                    Open seeded draft
                                  </a>
                                ) : (
                                  <button
                                    className="rounded-full border border-violet-400/20 bg-violet-300 px-3 py-2 text-zinc-950 transition hover:bg-violet-200 disabled:cursor-not-allowed disabled:bg-violet-300/60"
                                    type="button"
                                    onClick={() => void handleSeedNextDraft(event.id)}
                                    disabled={seedingEventId === event.id}
                                  >
                                    {seedingEventId === event.id ? 'Seeding…' : 'Seed next draft'}
                                  </button>
                                )
                              ) : null}
                            </>
                          ) : null}
                        </div>
                      </article>
                      );
                    })}
                  </div>
                </div>

                <section className="space-y-3 rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                  <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Workspace archive</p>
                  <div className="rounded-2xl border border-violet-400/20 bg-violet-400/10 p-4 text-sm leading-6 text-violet-50">
                    <p className="text-[11px] uppercase tracking-[0.25em] text-violet-100/80">Operator learning loop</p>
                    <p className="mt-2">{archiveLearningLoop}</p>
                  </div>

                  <form className="flex flex-wrap items-end gap-3 rounded-2xl border border-white/10 bg-white/[0.03] p-4" onSubmit={handleArchiveSearch}>
                    <label className="min-w-0 flex-1 space-y-2 text-sm">
                      <span className="text-zinc-300">Search archives</span>
                      <input
                        className="w-full rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-white/8"
                        placeholder="Search titles, locations, or notes"
                        value={archiveQuery}
                        onChange={(event) => setArchiveQuery(event.target.value)}
                      />
                    </label>
                    <button className="rounded-2xl bg-amber-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-amber-200 disabled:cursor-not-allowed disabled:bg-amber-300/60" type="submit" disabled={archiveSearching}>
                      {archiveSearching ? 'Searching…' : 'Search'}
                    </button>
                    <button
                      className="rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-zinc-200 transition hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-50"
                      type="button"
                      onClick={() => void handleArchiveReset()}
                      disabled={archiveSearching}
                    >
                      Reset
                    </button>
                  </form>

                  <div className="space-y-3">
                    {orderedArchives.length === 0 ? (
                      <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-5 text-sm text-zinc-400">
                        <p className="font-medium text-white">{normalizedArchiveQuery ? 'No archives matched your search' : 'No archives yet'}</p>
                        <p className="mt-1 leading-6">
                          {normalizedArchiveQuery ? 'Try a different search or reset the filter to show every private archive.' : 'Close an event to add its summary here.'}
                        </p>
                      </div>
                    ) : null}

                    {orderedArchives.map((archive) => (
                      <article key={archive.id} className="rounded-[1.5rem] border border-white/10 bg-white/[0.03] p-4">
                        <div className="flex flex-wrap items-start justify-between gap-3">
                          <div>
                            <h3 className="text-lg font-medium text-white">{archive.title}</h3>
                            <p className="mt-1 text-sm text-zinc-400">Starts {formatDateTime(archive.startsAt)}</p>
                            <p className="mt-1 text-sm text-zinc-400">Location {archive.locationDisplay}</p>
                          </div>
                          <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-xs uppercase tracking-[0.25em] text-zinc-300">
                            {archive.noteCount === 1 ? '1 note' : `${archive.noteCount} notes`}
                          </span>
                        </div>

                        <p className="mt-3 text-sm font-medium text-zinc-200">{archive.seededEventId ? 'Seeded draft ready' : 'No seeded draft yet'}</p>

                        <div className="mt-4 flex flex-wrap gap-2 text-sm">
                          <a className="rounded-full bg-white px-3 py-2 font-medium text-zinc-950 transition hover:bg-zinc-200" href={`/events/${archive.eventId}`}>
                            Open archive
                          </a>
                          {workspace?.role === 'owner' ? (
                            archive.seededEventId ? (
                              <a className="rounded-full border border-white/10 bg-white/5 px-3 py-2 text-zinc-200 transition hover:bg-white/10" href={`/events/${archive.seededEventId}`}>
                                Open seeded draft
                              </a>
                            ) : (
                              <button
                                className="rounded-full border border-violet-400/20 bg-violet-300 px-3 py-2 text-zinc-950 transition hover:bg-violet-200 disabled:cursor-not-allowed disabled:bg-violet-300/60"
                                type="button"
                                onClick={() => void handleSeedNextDraft(archive.eventId)}
                                disabled={seedingEventId === archive.eventId}
                              >
                                {seedingEventId === archive.eventId ? 'Seeding…' : 'Seed next draft'}
                              </button>
                            )
                          ) : null}
                        </div>
                      </article>
                    ))}
                  </div>
                </section>

                {templates !== null ? (
                  <section className="space-y-4 rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                    <div>
                      <p className="text-xs uppercase tracking-[0.3em] text-violet-300">Event templates</p>
                      <p className="mt-2 text-sm leading-6 text-zinc-400">Private planning memory for repeatable event setup.</p>
                    </div>

                    {templates.length === 0 ? (
                      <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-5 text-sm text-zinc-400">
                        <p className="font-medium text-white">No templates yet.</p>
                        <p className="mt-1">Save one from an event or create a new template below.</p>
                      </div>
                    ) : (
                      <div className="space-y-3">
                        {templates.map((template) => (
                          <article key={template.id} className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                            <div className="flex flex-wrap items-start justify-between gap-3">
                              <div>
                                <p className="text-lg font-medium text-white">{template.name}</p>
                                <p className="mt-1 text-sm text-zinc-400">{template.title}</p>
                              </div>

                              {workspace.role === 'owner' ? (
                                <div className="flex flex-wrap gap-2 text-xs uppercase tracking-[0.25em]">
                                  <button
                                    className="rounded-full border border-white/10 bg-white/5 px-3 py-1 text-zinc-200 transition hover:bg-white/10"
                                    type="button"
                                    onClick={() => editTemplate(template)}
                                  >
                                    Edit
                                  </button>
                                  <button
                                    className="rounded-full border border-rose-400/20 bg-rose-300 px-3 py-1 text-zinc-950 transition hover:bg-rose-200 disabled:cursor-not-allowed disabled:bg-rose-300/60"
                                    type="button"
                                    onClick={() => void handleTemplateDelete(template.id)}
                                    disabled={templateDeletingId === template.id}
                                  >
                                    {templateDeletingId === template.id ? 'Deleting…' : 'Delete'}
                                  </button>
                                </div>
                              ) : null}
                            </div>

                            <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                              <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">{template.locationDisplay || 'No location set'}</span>
                              <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">{templatePricingLabel(template)}</span>
                              <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">{template.ticketAllocation} tickets</span>
                            </div>

                            <p className="mt-3 text-sm leading-6 text-zinc-300">
                              <span className="text-zinc-500">Private note:</span> {template.privateNotes || 'No private note yet.'}
                            </p>

                            {template.publicDescription ? <p className="mt-3 text-sm leading-6 text-zinc-400">{template.publicDescription}</p> : null}
                          </article>
                        ))}
                      </div>
                    )}

                    {workspace.role === 'owner' ? (
                      <form className="space-y-4 rounded-2xl border border-white/10 bg-white/[0.03] p-4" onSubmit={handleTemplateSubmit}>
                        <div className="flex flex-wrap items-center justify-between gap-3">
                          <p className="text-xs uppercase tracking-[0.3em] text-zinc-500">{editingTemplateId ? 'Edit template' : 'Add template'}</p>
                          {editingTemplateId ? (
                            <button className="rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs uppercase tracking-[0.25em] text-zinc-200 transition hover:bg-white/10" type="button" onClick={resetTemplateEditor}>
                              Cancel
                            </button>
                          ) : null}
                        </div>

                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Name</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-violet-300/60 focus:bg-zinc-950/80"
                            value={templateForm.name}
                            onChange={(event) => setTemplateForm((current) => ({ ...current, name: event.target.value }))}
                            required
                          />
                        </label>

                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Title</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-violet-300/60 focus:bg-zinc-950/80"
                            value={templateForm.title}
                            onChange={(event) => setTemplateForm((current) => ({ ...current, title: event.target.value }))}
                            required
                          />
                        </label>

                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Location</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-violet-300/60 focus:bg-zinc-950/80"
                            value={templateForm.locationDisplay}
                            onChange={(event) => setTemplateForm((current) => ({ ...current, locationDisplay: event.target.value }))}
                          />
                        </label>

                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Public description</span>
                          <textarea
                            className="min-h-28 w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-violet-300/60 focus:bg-zinc-950/80"
                            value={templateForm.publicDescription}
                            onChange={(event) => setTemplateForm((current) => ({ ...current, publicDescription: event.target.value }))}
                          />
                        </label>

                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Ticket allocation</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-violet-300/60 focus:bg-zinc-950/80"
                            type="number"
                            min="0"
                            step="1"
                            value={templateForm.ticketAllocation}
                            onChange={(event) => setTemplateForm((current) => ({ ...current, ticketAllocation: event.target.value }))}
                          />
                        </label>

                        <fieldset className="rounded-[1.5rem] border border-white/10 bg-white/5 p-4">
                          <div className="flex flex-wrap items-start justify-between gap-3">
                            <div>
                              <p className="text-xs uppercase tracking-[0.3em] text-violet-300">Pricing</p>
                              <h2 className="mt-2 text-lg font-semibold text-white">Free or fixed paid tickets</h2>
                            </div>
                            <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-300">
                              USD only
                            </span>
                          </div>

                          <div className="mt-4 grid gap-3 sm:grid-cols-2">
                            <label className={`cursor-pointer rounded-2xl border p-4 transition ${templateForm.pricingMode === 'free' ? 'border-violet-300/40 bg-violet-300/10 text-white' : 'border-white/10 bg-white/5 text-zinc-300 hover:bg-white/8'}`}>
                              <input
                                className="sr-only"
                                type="radio"
                                name="templatePricingMode"
                                value="free"
                                checked={templateForm.pricingMode === 'free'}
                                onChange={() => setTemplateForm((current) => ({ ...current, pricingMode: 'free', ticketPriceDollars: '0.00' }))}
                              />
                              <p className="text-sm font-semibold">Free reservation</p>
                              <p className="mt-1 text-sm leading-6 text-current/70">Use this for no-cost plans.</p>
                            </label>

                            <label className={`cursor-pointer rounded-2xl border p-4 transition ${templateForm.pricingMode === 'fixed' ? 'border-violet-300/40 bg-violet-300/10 text-white' : 'border-white/10 bg-white/5 text-zinc-300 hover:bg-white/8'}`}>
                              <input
                                className="sr-only"
                                type="radio"
                                name="templatePricingMode"
                                value="fixed"
                                checked={templateForm.pricingMode === 'fixed'}
                                onChange={() => setTemplateForm((current) => ({ ...current, pricingMode: 'fixed' }))}
                              />
                              <p className="text-sm font-semibold">Fixed paid ticket</p>
                              <p className="mt-1 text-sm leading-6 text-current/70">Use a saved USD price for paid plans.</p>
                            </label>
                          </div>

                          {templateForm.pricingMode === 'fixed' ? (
                            <label className="mt-4 block space-y-2 text-sm">
                              <span className="text-zinc-300">Price in USD</span>
                              <input
                                className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-violet-300/60 focus:bg-zinc-950/80"
                                type="number"
                                min="0.5"
                                step="0.01"
                                inputMode="decimal"
                                value={templateForm.ticketPriceDollars}
                                onChange={(event) => setTemplateForm((current) => ({ ...current, ticketPriceDollars: event.target.value }))}
                                required
                              />
                            </label>
                          ) : null}
                        </fieldset>

                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Private notes</span>
                          <textarea
                            className="min-h-32 w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-violet-300/60 focus:bg-zinc-950/80"
                            value={templateForm.privateNotes}
                            onChange={(event) => setTemplateForm((current) => ({ ...current, privateNotes: event.target.value }))}
                            placeholder="Run-of-show notes stay private."
                          />
                        </label>

                        <button className="rounded-2xl bg-violet-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-violet-200 disabled:cursor-not-allowed disabled:bg-violet-300/60" type="submit" disabled={templateSubmitting}>
                          {templateSubmitting ? 'Saving…' : editingTemplateId ? 'Save template' : 'Add template'}
                        </button>

                        {templateNotice ? <p className="rounded-2xl border border-emerald-400/20 bg-emerald-400/10 px-4 py-3 text-sm text-emerald-200">{templateNotice}</p> : null}
                      </form>
                    ) : null}
                  </section>
                ) : null}

                {contacts !== null && !contactsDenied ? (
                  <section className="space-y-4 rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                    <div>
                      <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Contacts</p>
                      <p className="mt-2 text-sm leading-6 text-zinc-400">Private memory for people you want to remember across events.</p>
                    </div>

                    {visibleContacts.length === 0 ? (
                      <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-5 text-sm text-zinc-400">
                        <p className="font-medium text-white">No contacts yet. Add people you want to remember across events.</p>
                      </div>
                    ) : (
                      <div className="space-y-3">
                        {visibleContacts.map((contact) => (
                          <article key={contact.id} className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                            <div className="flex flex-wrap items-start justify-between gap-3">
                              <div>
                                <p className="text-lg font-medium text-white">{contact.displayName}</p>
                                <p className="mt-1 text-sm text-zinc-400">
                                  {[contact.email, contact.phone].filter(Boolean).join(' · ') || 'No contact details'}
                                </p>
                              </div>
                              {workspace.role === 'owner' ? (
                                <button className="rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs uppercase tracking-[0.25em] text-zinc-200 transition hover:bg-white/10" type="button" onClick={() => editContact(contact)}>
                                  Edit
                                </button>
                              ) : null}
                            </div>

                            {contact.tags.length > 0 ? (
                              <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                                {contact.tags.map((tag) => (
                                  <span key={tag} className="rounded-full border border-white/10 bg-white/5 px-3 py-1 text-zinc-300">
                                    {tag}
                                  </span>
                                ))}
                              </div>
                            ) : null}

                            <p className="mt-3 text-sm leading-6 text-zinc-300">
                              <span className="text-zinc-500">Private note:</span> {contact.notes || 'No private note yet.'}
                            </p>
                          </article>
                        ))}
                      </div>
                    )}

                    {reminders !== null ? (
                      <section className="space-y-4 rounded-[1.75rem] border border-fuchsia-400/20 bg-zinc-950/90 p-6 shadow-2xl shadow-black/20">
                        <div className="flex flex-wrap items-start justify-between gap-4">
                          <div>
                            <p className="text-xs uppercase tracking-[0.3em] text-fuchsia-300">Reminder activity</p>
                            <h3 className="mt-2 text-2xl font-semibold text-white">
                              {reminders === undefined ? 'Loading reminders…' : `${visibleReminders.length} reminder${visibleReminders.length === 1 ? '' : 's'}`}
                            </h3>
                            <p className="mt-2 text-sm leading-6 text-zinc-400">
                              Private reminder sweeps stay here for operators without exposing commitment descriptions, staffing notes, or public event copy.
                            </p>
                          </div>

                          {workspace.role === 'owner' ? (
                            <button
                              className="rounded-2xl border border-fuchsia-400/30 bg-fuchsia-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-fuchsia-200 disabled:cursor-not-allowed disabled:bg-fuchsia-300/60"
                              type="button"
                              onClick={() => void handleRunReminderSweep()}
                              disabled={reminderSweepRunning || reminders === undefined}
                            >
                              {reminderSweepRunning ? 'Sweeping…' : 'Run reminder sweep'}
                            </button>
                          ) : null}
                        </div>

                        {reminders === undefined ? (
                          <p className="text-sm leading-6 text-zinc-400">Loading reminder activity…</p>
                        ) : visibleReminders.length > 0 ? (
                          <div className="space-y-3">
                            {visibleReminders.map((reminder) => (
                              <article key={reminder.id} className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                                <div className="flex flex-wrap items-start justify-between gap-3">
                                  <div>
                                    <p className="text-sm font-semibold text-white">{reminder.subject}</p>
                                    <p className="mt-1 text-sm text-zinc-400">
                                      {reminder.recipientEmail} · {reminder.reminderType}
                                    </p>
                                  </div>
                                  <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-200">
                                    {reminder.status}
                                  </span>
                                </div>

                                <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                                  <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Due {formatDateTime(reminder.dueAt)}</span>
                                  <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Preview {reminder.preview}</span>
                                  <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Created {formatDateTime(reminder.createdAt)}</span>
                                </div>
                              </article>
                            ))}
                          </div>
                        ) : (
                          <p className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">No reminder activity yet.</p>
                        )}
                      </section>
                    ) : null}

                    {workspace.role === 'owner' ? (
                      <form className="space-y-4 rounded-2xl border border-white/10 bg-white/[0.03] p-4" onSubmit={handleContactSubmit}>
                        <div className="flex flex-wrap items-center justify-between gap-3">
                          <p className="text-xs uppercase tracking-[0.3em] text-zinc-500">{editingContactId ? 'Edit contact' : 'Add contact'}</p>
                          {editingContactId ? (
                            <button className="rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs uppercase tracking-[0.25em] text-zinc-200 transition hover:bg-white/10" type="button" onClick={resetContactEditor}>
                              Cancel
                            </button>
                          ) : null}
                        </div>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Display name</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-zinc-950/80"
                            value={contactForm.displayName}
                            onChange={(event) => setContactForm((current) => ({ ...current, displayName: event.target.value }))}
                            required
                          />
                        </label>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Email</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-zinc-950/80"
                            type="email"
                            value={contactForm.email}
                            onChange={(event) => setContactForm((current) => ({ ...current, email: event.target.value }))}
                          />
                        </label>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Phone</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-zinc-950/80"
                            value={contactForm.phone}
                            onChange={(event) => setContactForm((current) => ({ ...current, phone: event.target.value }))}
                          />
                        </label>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Tags</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-zinc-950/80"
                            placeholder="trusted, door"
                            value={contactForm.tags}
                            onChange={(event) => setContactForm((current) => ({ ...current, tags: event.target.value }))}
                          />
                        </label>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Notes</span>
                          <textarea
                            className="min-h-28 w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-zinc-950/80"
                            value={contactForm.notes}
                            onChange={(event) => setContactForm((current) => ({ ...current, notes: event.target.value }))}
                          />
                        </label>
                        <button className="rounded-2xl bg-amber-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-amber-200" type="submit">
                          {editingContactId ? 'Save contact' : 'Add contact'}
                        </button>
                      </form>
                    ) : null}
                  </section>
                ) : null}

                {commitments !== null && !commitmentsDenied ? (
                  <section className="space-y-4 rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                    <div>
                      <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Commitments</p>
                      <p className="mt-2 text-sm leading-6 text-zinc-400">Track private promises, due dates, and follow-up status across the workspace.</p>
                    </div>

                    <div className="grid gap-3 text-sm sm:grid-cols-3">
                      <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Open</p>
                        <p className="mt-2 text-2xl font-semibold text-white">{commitmentCounts.open}</p>
                      </div>
                      <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Done</p>
                        <p className="mt-2 text-2xl font-semibold text-white">{commitmentCounts.done}</p>
                      </div>
                      <div className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Cancelled</p>
                        <p className="mt-2 text-2xl font-semibold text-white">{commitmentCounts.cancelled}</p>
                      </div>
                    </div>

                    {visibleCommitments.length === 0 ? (
                      <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-5 text-sm text-zinc-400">
                        <p className="font-medium text-white">No commitments yet.</p>
                      </div>
                    ) : (
                      <div className="space-y-3">
                        {visibleCommitments.map((commitment) => (
                          <article key={commitment.id} className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                            <div className="flex flex-wrap items-start justify-between gap-3">
                              <div>
                                <p className="text-lg font-medium text-white">{commitment.title}</p>
                                <p className="mt-1 text-sm text-zinc-400">
                                  {commitment.dueAt ? `Due ${formatDateTime(commitment.dueAt)}` : 'No due date'}
                                  {commitment.eventId ? ` · ${eventTitleById.get(commitment.eventId) ?? 'Workspace event'}` : ' · Workspace level'}
                                </p>
                              </div>
                              <span className={`rounded-full border px-3 py-1 text-xs uppercase tracking-[0.25em] ${commitmentStatusTone(commitment.status)}`}>
                                {commitmentStatusLabel(commitment.status)}
                              </span>
                            </div>
                            <p className="mt-3 text-sm leading-6 text-zinc-300">{commitment.description || 'No private description yet.'}</p>

                            {workspace.role === 'owner' ? (
                              <div className="mt-4 flex flex-wrap gap-2 text-sm">
                                <button
                                  className="rounded-full border border-emerald-400/20 bg-emerald-300 px-3 py-2 font-medium text-zinc-950 transition hover:bg-emerald-200"
                                  type="button"
                                  onClick={() => void handleCommitmentStatus(commitment.id, 'done')}
                                >
                                  Mark done
                                </button>
                                <button
                                  className="rounded-full border border-white/10 bg-white/5 px-3 py-2 text-zinc-200 transition hover:bg-white/10"
                                  type="button"
                                  onClick={() => void handleCommitmentStatus(commitment.id, 'open')}
                                >
                                  Reopen
                                </button>
                                <button
                                  className="rounded-full border border-rose-400/20 bg-rose-300 px-3 py-2 font-medium text-zinc-950 transition hover:bg-rose-200"
                                  type="button"
                                  onClick={() => void handleCommitmentStatus(commitment.id, 'cancelled')}
                                >
                                  Cancel
                                </button>
                              </div>
                            ) : null}
                          </article>
                        ))}
                      </div>
                    )}

                    {workspace.role === 'owner' ? (
                      <form className="space-y-4 rounded-2xl border border-white/10 bg-white/[0.03] p-4" onSubmit={handleCommitmentSubmit}>
                        <p className="text-xs uppercase tracking-[0.3em] text-zinc-500">Add commitment</p>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Title</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-zinc-950/80"
                            value={commitmentForm.title}
                            onChange={(event) => setCommitmentForm((current) => ({ ...current, title: event.target.value }))}
                            required
                          />
                        </label>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Description</span>
                          <textarea
                            className="min-h-28 w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-zinc-950/80"
                            value={commitmentForm.description}
                            onChange={(event) => setCommitmentForm((current) => ({ ...current, description: event.target.value }))}
                          />
                        </label>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Due at</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-zinc-950/80"
                            type="datetime-local"
                            value={commitmentForm.dueAt}
                            onChange={(event) => setCommitmentForm((current) => ({ ...current, dueAt: event.target.value }))}
                          />
                        </label>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Event</span>
                          <select
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-zinc-950/80"
                            value={commitmentForm.eventId}
                            onChange={(event) => setCommitmentForm((current) => ({ ...current, eventId: event.target.value }))}
                          >
                            <option value="">Workspace only</option>
                            {orderedEvents.map((event) => (
                              <option key={event.id} value={event.id}>
                                {event.title}
                              </option>
                            ))}
                          </select>
                        </label>
                        <button className="rounded-2xl bg-amber-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-amber-200" type="submit">
                          Add commitment
                        </button>
                      </form>
                    ) : null}
                  </section>
                ) : null}
              </div>

              <aside className="space-y-6">
                <form id="invite-member" className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6" onSubmit={handleInvite}>
                  <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Invite member</p>
                  <p className="mt-2 text-sm leading-6 text-zinc-400">Send an invite without reloading the page; the new row appears below as soon as it lands.</p>
                  <label className="mt-4 block space-y-2 text-sm">
                    <span className="text-zinc-300">Email</span>
                    <input
                      className="w-full rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-white/8"
                      type="email"
                      autoComplete="email"
                      required
                      value={inviteEmail}
                      onChange={(event) => {
                        setInviteEmail(event.target.value);
                        setInviteNotice(null);
                      }}
                    />
                  </label>
                  <button className="mt-4 w-full rounded-2xl bg-white px-4 py-3 font-medium text-zinc-950 transition hover:bg-zinc-200 disabled:cursor-not-allowed disabled:bg-white/70" type="submit" disabled={sendingInvite}>
                    {sendingInvite ? 'Sending…' : 'Send invite'}
                  </button>
                  {inviteNotice ? <p className="mt-3 rounded-2xl border border-emerald-400/20 bg-emerald-400/10 px-4 py-3 text-sm text-emerald-200">{inviteNotice}</p> : null}
                </form>

                <section className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                  <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Invitations</p>
                  <p className="mt-2 text-sm leading-6 text-zinc-400">Pending vs accepted, with open links when the token is available.</p>

                  <div className="mt-4 space-y-3 text-sm">
                    {workspace.invitations.length === 0 ? (
                      <div className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-zinc-400">
                        No invitations yet. Send one above to start building the crew list.
                      </div>
                    ) : null}

                    {workspace.invitations.map((invitation) => {
                      const accepted = invitation.acceptedAt !== null;

                      return (
                        <article key={invitation.id} className="rounded-2xl border border-white/10 bg-white/5 p-4">
                          <div className="flex items-start justify-between gap-3">
                            <div>
                              <p className="font-medium text-white">{invitation.email}</p>
                              <p className="mt-1 text-zinc-400">{roleLabel(invitation.role)}</p>
                            </div>
                            <span className={`rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.25em] ${accepted ? 'border-emerald-400/20 bg-emerald-400/10 text-emerald-200' : 'border-amber-400/20 bg-amber-400/10 text-amber-200'}`}>
                              {accepted ? 'Accepted' : 'Pending'}
                            </span>
                          </div>

                          {invitation.acceptedAt ? <p className="mt-3 text-zinc-400">Accepted {formatShortDateTime(invitation.acceptedAt)}</p> : <p className="mt-3 text-zinc-500">Waiting for the invite to be accepted.</p>}

                          {invitation.token ? (
                            <a className="mt-3 inline-flex rounded-full bg-amber-300 px-3 py-2 text-xs font-medium text-zinc-950 transition hover:bg-amber-200" href={`/invite/${invitation.token}`}>
                              Open invite
                            </a>
                          ) : null}
                        </article>
                      );
                    })}
                  </div>
                </section>

                <section className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                  <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Workspace access</p>
                  <p className="mt-2 text-sm leading-6 text-zinc-400">One person can operate multiple Workspaces. Use this switcher to jump between them.</p>
                  <div className="mt-4 space-y-2 text-sm text-zinc-400">
                    {workspaceSummaries.map((summary) => (
                      <a
                        key={summary.id}
                        className={`flex items-center justify-between gap-3 rounded-2xl border px-4 py-3 transition ${
                          summary.id === workspace.id
                            ? 'border-amber-300/40 bg-amber-300/10 shadow-[0_0_0_1px_rgba(252,211,77,0.12)]'
                            : 'border-white/10 bg-white/5 hover:bg-white/10'
                        }`}
                        href={`/workspace?workspaceId=${summary.id}`}
                        aria-current={summary.id === workspace.id ? 'page' : undefined}
                      >
                        <span className="min-w-0">
                          <span className={`block truncate ${summary.id === workspace.id ? 'text-white' : 'text-zinc-200'}`}>{summary.name}</span>
                          <span className="mt-1 block text-xs uppercase tracking-[0.25em] text-zinc-500">{roleLabel(summary.role)}</span>
                        </span>
                        <span
                          className={`shrink-0 rounded-full border px-3 py-1 text-[11px] uppercase tracking-[0.25em] ${
                            summary.id === workspace.id
                              ? 'border-amber-300/40 bg-amber-300/10 text-amber-100'
                              : 'border-white/10 bg-black/20 text-zinc-300'
                          }`}
                        >
                          {summary.id === workspace.id ? 'Active' : 'Open'}
                        </span>
                      </a>
                    ))}
                  </div>
                  <p className="mt-4 text-xs leading-6 text-zinc-500">
                    The active Workspace is highlighted so you can move between rooms without losing your place.
                  </p>
                </section>

                {emailOutbox && emailOutbox.length > 0 ? (
                  <section className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                    <p className="text-xs uppercase tracking-[0.3em] text-fuchsia-300">Dev email outbox</p>
                    <p className="mt-2 text-sm leading-6 text-zinc-400">Development-only mailbox. It surfaces recent invite and ticket emails with quick links when available.</p>

                    <div className="mt-4 space-y-3">
                      {emailOutbox.map((email) => {
                        const inviteToken = extractInviteToken(email.body);
                        const ticketCode = extractTicketCode(email.body);

                        return (
                          <article key={email.id} className="rounded-2xl border border-white/10 bg-white/5 p-4 text-sm">
                            <div className="flex items-start justify-between gap-3">
                              <div>
                                <p className="font-medium text-white">{email.subject}</p>
                                <p className="mt-1 text-zinc-400">To {email.recipientEmail}</p>
                              </div>
                              <p className="text-xs uppercase tracking-[0.25em] text-zinc-500">{formatDateTime(email.createdAt)}</p>
                            </div>

                            <p className="mt-3 line-clamp-3 whitespace-pre-wrap text-zinc-300">{email.body}</p>

                            <div className="mt-4 flex flex-wrap gap-2 text-xs uppercase tracking-[0.25em] text-zinc-500">
                              <span>{email.relatedType}</span>
                              <span>{email.relatedId}</span>
                            </div>

                            {inviteToken ? (
                              <a className="mt-4 inline-flex rounded-full bg-amber-300 px-3 py-2 text-xs font-medium text-zinc-950 transition hover:bg-amber-200" href={`/invite/${inviteToken}`}>
                                Open invite
                              </a>
                            ) : null}
                            {ticketCode ? (
                              <a className="mt-4 ml-2 inline-flex rounded-full border border-white/10 bg-white/5 px-3 py-2 text-xs font-medium text-zinc-200 transition hover:bg-white/10" href={`/tickets/${ticketCode}`}>
                                Open ticket
                              </a>
                            ) : null}
                          </article>
                        );
                      })}
                    </div>
                  </section>
                ) : null}
              </aside>
            </section>

            <p className="px-1 text-xs uppercase tracking-[0.3em] text-zinc-500">Workspace ID {workspaceId}</p>
          </>
        ) : null}
      </section>
    </main>
  );
}
