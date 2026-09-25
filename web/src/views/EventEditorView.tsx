import { useEffect, useMemo, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import { ApiError, api, patchJSON, postJSON } from '../api';
import { eventLifecycleLabel as statusLabel, isClosedEvent, isDraftEvent, isPublishedEvent } from '../modules/eventLifecycle/eventLifecycle';
import {
	buildCreateRunOfShowPayload,
	emptyRunOfShowForm,
	staffingKindLabel,
	staffingStatusLabel,
	staffingWindowLabel,
	sortRunOfShowItems,
} from '../modules/runOfShow/runOfShowModel';
import {
	applicationReviewStatusOptions,
	buildCommitmentCounts,
	buildPayload,
	buildRoleNameById,
	buildStaffingCounts,
	buildStaffingGroups,
	commitmentStatusLabel,
	commitmentStatusTone,
	emptyCommitmentForm,
	emptyForm,
	emptySettlementAdjustmentForm,
	formatDateTime,
	formatMoney,
	formatSignedMoney,
	formFromEvent,
	formsMatch,
	fromInputValue,
	getWorkspaceId,
	isNewEvent,
	pricingSummary,
	priceInCents,
	reportEndOfNightCopy,
	settlementAdjustmentsEmptyCopy,
	settlementAdjustmentsLockedCopy,
	settlementLockedCopy,
	settlementStatusLabel,
	sortCommitments,
	sortTemplates,
	toRfc3339DateTime,
	type CommitmentFormState,
	type FormState,
	type SettlementAdjustmentFormState,
} from '../modules/eventEditor/eventEditorModel';
import {
	loadEventEditorArchive,
	loadEventEditorCommitments,
	loadEventEditorEvent,
	loadEventEditorNotifications,
	loadEventEditorParticipants,
	loadEventEditorReport,
	loadEventEditorReminders,
	loadEventEditorRoleApplications,
	loadEventEditorSettlement,
	loadEventEditorStaffing,
	loadEventEditorTemplates,
	loadEventEditorWorkspace,
} from '../modules/eventEditor/eventEditorLoaders';
import { canDownloadSettlementExport, downloadSettlementExport, downloadSettlementReport } from '../modules/eventEditor/settlementExport';
import type {
  CommitmentDTO,
  CurrentWorkspaceDTO,
  EventArchiveDTO,
  EventDTO,
  EventTemplateDTO,
  EventParticipantDTO,
  EventReportDTO,
  EventRoleApplicationDTO,
  EventRoleDTO,
  EventSettlementDTO,
  EventStaffingItemDTO,
  ReminderEventDTO,
  NotificationEventDTO,
} from '../domain';

function statusTone(status: EventDTO['status']) {
  switch (status) {
    case 'draft':
      return 'border-amber-400/30 bg-amber-400/10 text-amber-200';
    case 'published':
      return 'border-emerald-400/30 bg-emerald-400/10 text-emerald-200';
    case 'end_of_night':
      return 'border-fuchsia-400/30 bg-fuchsia-400/10 text-fuchsia-200';
  }
}

function statusSummary(status: EventDTO['status']) {
  switch (status) {
    case 'draft':
      return 'Private until the checklist is complete and the public page goes live.';
    case 'published':
      return 'Live now. Keep the public page handy and end the night when the door closes.';
    case 'end_of_night':
      return 'Closed out. Review the report and jump back to the workspace when you are done.';
  }
}


export function EventEditorView({ eventId }: { eventId: string }) {
  const creating = isNewEvent(eventId);
  const workspaceId = useMemo(getWorkspaceId, []);
  const [event, setEvent] = useState<EventDTO | null>(null);
  const [report, setReport] = useState<EventReportDTO | null>(null);
  const [archive, setArchive] = useState<EventArchiveDTO | null>(null);
  const [archiveNoteBody, setArchiveNoteBody] = useState('');
  const [archiveLoading, setArchiveLoading] = useState(false);
  const [archiveSubmitting, setArchiveSubmitting] = useState(false);
  const [form, setForm] = useState<FormState>(emptyForm);
  const [initialForm, setInitialForm] = useState<FormState>(emptyForm);
  const [loading, setLoading] = useState(!creating);
  const [saving, setSaving] = useState(false);
  const [actioning, setActioning] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [settlement, setSettlement] = useState<EventSettlementDTO | null>(null);
  const [settlementForm, setSettlementForm] = useState<SettlementAdjustmentFormState>(emptySettlementAdjustmentForm);
  const [settlementSubmitting, setSettlementSubmitting] = useState(false);
  const [settlementFinalizing, setSettlementFinalizing] = useState(false);
  const [currentWorkspace, setCurrentWorkspace] = useState<CurrentWorkspaceDTO | null>(null);
  const [roles, setRoles] = useState<EventRoleDTO[] | null>(null);
  const [applications, setApplications] = useState<EventRoleApplicationDTO[] | null>(null);
  const [applicationReviewDrafts, setApplicationReviewDrafts] = useState<Record<string, EventRoleApplicationDTO['status']>>({});
  const [reviewingApplicationId, setReviewingApplicationId] = useState<string | null>(null);
  const [participants, setParticipants] = useState<EventParticipantDTO[] | null>(null);
  const [staffingItems, setStaffingItems] = useState<EventStaffingItemDTO[] | null>(null);
  const [staffingLoading, setStaffingLoading] = useState(false);
  const [staffingForm, setStaffingForm] = useState(emptyRunOfShowForm());
  const [staffingActioningId, setStaffingActioningId] = useState<string | null>(null);
  const [notifications, setNotifications] = useState<NotificationEventDTO[] | null | undefined>(undefined);
  const [notificationsRefreshTick, setNotificationsRefreshTick] = useState(0);
  const [commitments, setCommitments] = useState<CommitmentDTO[] | null>(null);
  const [commitmentsDenied, setCommitmentsDenied] = useState(false);
  const [commitmentForm, setCommitmentForm] = useState<CommitmentFormState>(emptyCommitmentForm());
  const [commitmentSubmitting, setCommitmentSubmitting] = useState(false);
  const [commitmentActioningId, setCommitmentActioningId] = useState<string | null>(null);
  const [templates, setTemplates] = useState<EventTemplateDTO[] | null>(null);
  const [templateSelectionId, setTemplateSelectionId] = useState('');
  const [templateApplying, setTemplateApplying] = useState(false);
  const [templateSavingFromEvent, setTemplateSavingFromEvent] = useState(false);
  const [reminders, setReminders] = useState<ReminderEventDTO[] | null | undefined>(undefined);
  const [reportError, setReportError] = useState<string | null>(null);
  const [reportRefreshTick, setReportRefreshTick] = useState(0);
  const [workspaceError, setWorkspaceError] = useState<string | null>(null);
  const [workspaceRefreshTick, setWorkspaceRefreshTick] = useState(0);
  const [settlementExporting, setSettlementExporting] = useState(false);
  const [reportAccessDenied, setReportAccessDenied] = useState(false);
  const [settlementAccessDenied, setSettlementAccessDenied] = useState(false);
  const commitmentsRevisionRef = useRef(0);

  const hasWorkspace = workspaceId !== '';
  const closed = isClosedEvent(event?.status);
  const pricingLocked = (event?.reservedCount ?? 0) > 0 || closed;
  const settlementFinalized = settlement?.status === 'finalized';
  const settlementOpen = settlement?.status === 'open';
  const financeWorkspaceID = event?.workspaceId ?? (creating ? workspaceId : undefined);
  const canManageFinance = canDownloadSettlementExport(currentWorkspace?.id, financeWorkspaceID, currentWorkspace?.role);
  const canExportSettlement = Boolean(event) && canManageFinance;
  const canManageSettlement = canExportSettlement;
  const canEditPricing = canManageFinance;
  const canSelectFreePricing = creating || canEditPricing;
  const financeAccessDenied = reportAccessDenied || settlementAccessDenied;
  const canManageArchive = isClosedEvent(event?.status) && currentWorkspace?.role === 'owner' && currentWorkspace?.id === event?.workspaceId;
  const canReviewApplications = currentWorkspace?.role === 'owner' && currentWorkspace?.id === event?.workspaceId;
  const canViewNotificationActivity = currentWorkspace?.id === event?.workspaceId && (currentWorkspace?.role === 'owner' || currentWorkspace?.role === 'member');
  const canViewReminderActivity = currentWorkspace?.id === event?.workspaceId && (currentWorkspace?.role === 'owner' || currentWorkspace?.role === 'member');
  const loadedNotificationActivity = Array.isArray(notifications) ? notifications : null;
  const loadedReminderActivity = Array.isArray(reminders) ? reminders : null;
  const applicationsReady = roles !== null && applications !== null;
  const participantsReady = participants !== null;
  const staffingReady = staffingLoading || staffingItems !== null;
  const canManageStaffing = currentWorkspace?.role === 'owner' && currentWorkspace?.id === event?.workspaceId && !closed;
  const roleNameById = useMemo(() => buildRoleNameById(roles), [roles]);
  const staffingAssigneeOptions = useMemo(
    () => ({
      members: (currentWorkspace?.members ?? []).map((member) => ({
        value: `member:${member.id}`,
        label: member.displayName ?? member.email,
      })),
      participants: (participants ?? []).map((participant) => ({
        value: `participant:${participant.applicationId}`,
        label: `${participant.applicantName} • ${participant.roleName} • ${participant.status}`,
      })),
    }),
    [currentWorkspace?.members, participants],
  );
  const staffingCounts = useMemo(() => buildStaffingCounts(staffingItems), [staffingItems]);
  const staffingGroups: Array<{ kind: 'task' | 'shift'; label: string; items: EventStaffingItemDTO[] }> = useMemo(() => buildStaffingGroups(staffingItems), [staffingItems]);
  const visibleCommitments = useMemo(() => (commitments ? sortCommitments(commitments) : []), [commitments]);
  const commitmentCounts = useMemo(() => buildCommitmentCounts(visibleCommitments), [visibleCommitments]);
  const dirty = useMemo(() => !formsMatch(form, initialForm), [form, initialForm]);

  useEffect(() => {
    let cancelled = false;

    async function load() {
      if (creating) {
        setLoading(false);
        setSettlement(null);
        setSettlementForm(emptySettlementAdjustmentForm());
        return;
      }

      setLoading(true);
      setError(null);
      setSettlement(null);

      try {
        const loadedEvent = await loadEventEditorEvent(api, eventId);
        if (cancelled) return;

        setEvent(loadedEvent);

        const loadedForm = formFromEvent(loadedEvent);
        setForm(loadedForm);
        setInitialForm(loadedForm);
        setReport(null);

      } catch (caught) {
        if (!cancelled) {
          setError(caught instanceof Error ? caught.message : 'Unable to load event');
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
  }, [creating, eventId]);

  useEffect(() => {
    let cancelled = false;
    setReportError(null);
    if (!event || !isClosedEvent(event.status)) {
      setReport(null);
      setReportAccessDenied(false);
      return;
    }
    setReport((current) => current?.eventId === event.id ? current : null);
    setReportAccessDenied(false);

    void loadEventEditorReport(api, event.id).then((loaded) => {
      if (!cancelled) {
        setReport(loaded.data);
        setReportAccessDenied(loaded.denied);
      }
    }).catch((caught) => {
      if (!cancelled) setReportError(caught instanceof Error ? caught.message : 'Unable to load report');
    });

    return () => { cancelled = true; };
  }, [event?.id, event?.status, reportRefreshTick]);

  useEffect(() => {
    let cancelled = false;

    async function loadArchive() {
      if (creating || !event || !isClosedEvent(event.status)) {
        setArchive(null);
        setArchiveNoteBody('');
        setArchiveLoading(false);
        return;
      }

      setArchiveLoading(true);
      setArchiveNoteBody('');

      try {
        const loadedArchive = await loadEventEditorArchive(api, event.id);
        if (!cancelled) {
          setArchive(loadedArchive);
        }
      } catch (caught) {
        if (cancelled) return;

        setError(caught instanceof Error ? caught.message : 'Unable to load archive');
      } finally {
        if (!cancelled) {
          setArchiveLoading(false);
        }
      }
    }

    void loadArchive();

    return () => {
      cancelled = true;
    };
  }, [creating, event?.id, event?.status]);

  useEffect(() => {
    if (creating) {
      const blank = emptyForm();
      setEvent(null);
      setReport(null);
      setSettlement(null);
      setStaffingItems(null);
      setStaffingLoading(false);
      setStaffingForm(emptyRunOfShowForm());
      setStaffingActioningId(null);
      setForm(blank);
      setInitialForm(blank);
      setSettlementForm(emptySettlementAdjustmentForm());
    }
  }, [creating]);

  useEffect(() => {
    let cancelled = false;

    async function loadEventWorkspace() {
      setCurrentWorkspace(null);
      setWorkspaceError(null);
      const requestedWorkspaceID = event?.workspaceId ?? (creating ? workspaceId : '');
      if (!requestedWorkspaceID) {
        return;
      }

      try {
        const loaded = await loadEventEditorWorkspace(api, requestedWorkspaceID);
        if (!cancelled) setCurrentWorkspace(loaded);
      } catch (caught) {
        if (!cancelled) setWorkspaceError(caught instanceof Error ? caught.message : 'Unable to load workspace authority');
      }
    }

    void loadEventWorkspace();

    return () => {
      cancelled = true;
    };
  }, [creating, event?.workspaceId, workspaceId, workspaceRefreshTick]);

  useEffect(() => {
    let cancelled = false;

    async function loadRoleApplications() {
      if (creating || !event) {
        setRoles(null);
        setApplications(null);
        setApplicationReviewDrafts({});
        setReviewingApplicationId(null);
        return;
      }

      setRoles(null);
      setApplications(null);

      try {
        const loaded = await loadEventEditorRoleApplications(api, event.id);

        if (!cancelled) {
          setRoles(loaded.roles);
          setApplications(loaded.applications);
          setApplicationReviewDrafts({});
          setReviewingApplicationId(null);
        }
      } catch (caught) {
        if (!cancelled) {
          setError(caught instanceof Error ? caught.message : 'Unable to load role applications');
        }
      }
    }

    void loadRoleApplications();

    return () => {
      cancelled = true;
    };
  }, [creating, event?.id]);

  useEffect(() => {
    let cancelled = false;

    async function loadNotifications() {
      if (creating || !event || !canViewNotificationActivity) {
        setNotifications(null);
        return;
      }

      setNotifications(undefined);

      try {
        const loadedNotifications = await loadEventEditorNotifications(api, event.id);
        if (!cancelled) {
          setNotifications(loadedNotifications);
        }
      } catch (caught) {
        if (cancelled) return;

        if (caught instanceof ApiError && (caught.status === 403 || caught.status === 404)) {
          setNotifications(null);
          return;
        }

        setError(caught instanceof Error ? caught.message : 'Unable to load notification activity');
        setNotifications(null);
      }
    }

    void loadNotifications();

    return () => {
      cancelled = true;
    };
  }, [canViewNotificationActivity, creating, event?.id, notificationsRefreshTick]);

  useEffect(() => {
    let cancelled = false;

    async function loadReminders() {
      if (creating || !event || !canViewReminderActivity) {
        setReminders(null);
        return;
      }

      setReminders(undefined);

      try {
        const loadedReminders = await loadEventEditorReminders(api, event.id);
        if (!cancelled) {
          setReminders(loadedReminders);
        }
      } catch (caught) {
        if (cancelled) return;

        if (caught instanceof ApiError && (caught.status === 403 || caught.status === 404)) {
          setReminders(null);
          return;
        }

        setError(caught instanceof Error ? caught.message : 'Unable to load reminder activity');
        setReminders(null);
      }
    }

    void loadReminders();

    return () => {
      cancelled = true;
    };
  }, [canViewReminderActivity, creating, event?.id]);

  useEffect(() => {
    let cancelled = false;

    async function loadParticipants() {
      if (creating || !event) {
        setParticipants(null);
        return;
      }

      setParticipants(null);

      try {
        const loadedParticipants = await loadEventEditorParticipants(api, event.id);
        if (!cancelled) {
          setParticipants(loadedParticipants);
        }
      } catch (caught) {
        if (!cancelled) {
          setError(caught instanceof Error ? caught.message : 'Unable to load participant roster');
        }
      }
    }

    void loadParticipants();

    return () => {
      cancelled = true;
    };
  }, [creating, event?.id]);

  useEffect(() => {
    let cancelled = false;

    async function loadStaffing() {
      if (creating || !event) {
        setStaffingItems(null);
        setStaffingLoading(false);
        setStaffingForm(emptyRunOfShowForm());
        setStaffingActioningId(null);
        return;
      }

      setStaffingLoading(true);
      setStaffingItems(null);
      setStaffingForm(emptyRunOfShowForm());
      setStaffingActioningId(null);

      try {
        const loadedStaffing = await loadEventEditorStaffing(api, event.id);
        if (!cancelled) {
          setStaffingItems(loadedStaffing);
        }
      } catch (caught) {
        if (!cancelled) {
          setError(caught instanceof Error ? caught.message : 'Unable to load staffing board');
        }
      } finally {
        if (!cancelled) {
          setStaffingLoading(false);
        }
      }
    }

    void loadStaffing();

    return () => {
      cancelled = true;
    };
  }, [creating, event?.id]);

  useEffect(() => {
    let cancelled = false;

    async function loadSettlement() {
      if (creating || !event || !isClosedEvent(event.status)) {
        setSettlement(null);
        setSettlementAccessDenied(false);
        return;
      }
      setSettlementAccessDenied(false);

      try {
        const loadedSettlement = await loadEventEditorSettlement(api, event.id);
        if (!cancelled) {
          setSettlement(loadedSettlement.data);
          setSettlementAccessDenied(loadedSettlement.denied);
        }
      } catch (caught) {
        if (cancelled) return;

        setError(caught instanceof Error ? caught.message : 'Unable to load settlement');
      }
    }

    void loadSettlement();

    return () => {
      cancelled = true;
    };
  }, [creating, event?.id, event?.status]);

  useEffect(() => {
    let cancelled = false;

    async function loadCommitments() {
      const requestRevision = ++commitmentsRevisionRef.current;

      if (creating || !event) {
        setCommitments(null);
        setCommitmentsDenied(false);
        setCommitmentForm(emptyCommitmentForm());
        setCommitmentSubmitting(false);
        setCommitmentActioningId(null);
        return;
      }

      setCommitments(null);
      setCommitmentsDenied(false);
      setCommitmentForm(emptyCommitmentForm());
      setCommitmentSubmitting(false);
      setCommitmentActioningId(null);

      try {
        const loadedCommitments = await loadEventEditorCommitments(api, event.id);
        if (!cancelled && requestRevision === commitmentsRevisionRef.current) {
          setCommitmentsDenied(loadedCommitments.denied);
          setCommitments(loadedCommitments.commitments);
        }
      } catch (caught) {
        if (cancelled) return;

        if (requestRevision === commitmentsRevisionRef.current) {
          setError(caught instanceof Error ? caught.message : 'Unable to load commitments');
        }
      }
    }

    void loadCommitments();

    return () => {
      cancelled = true;
    };
  }, [creating, event?.id]);

  useEffect(() => {
    let cancelled = false;

    async function loadTemplates() {
      if (creating || !event || !isDraftEvent(event.status) || currentWorkspace?.id !== event.workspaceId || currentWorkspace?.role !== 'owner') {
        setTemplates(null);
        setTemplateSelectionId('');
        return;
      }

      try {
        const loadedTemplates = await loadEventEditorTemplates(api, event.workspaceId);
        if (!cancelled) {
          setTemplates(loadedTemplates);
          setTemplateSelectionId((current) => current || loadedTemplates?.[0]?.id || '');
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
  }, [creating, currentWorkspace?.id, currentWorkspace?.role, event?.workspaceId, event?.status]);

  async function persist() {
    const payload = buildPayload(form);

    if (creating) {
      if (!hasWorkspace) {
        throw new Error('workspaceId is required to create an event');
      }

      const created = await postJSON<EventDTO>(`/api/workspaces/${workspaceId}/events`, payload);
      window.location.href = `/events/${created.id}`;
      return;
    }

    if (!dirty) {
      setMessage('Nothing to save yet');
      return;
    }

    const updated = await patchJSON<EventDTO>(`/api/events/${eventId}`, payload);
    const updatedForm = formFromEvent(updated);
    setEvent(updated);
    setForm(updatedForm);
    setInitialForm(updatedForm);
    setMessage('Saved');
  }

  async function handleSubmit(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    if (saving || actioning) return;
    setSaving(true);
    setMessage(null);
    setError(null);

    try {
      await persist();
    } catch (caught) {
      if (caught instanceof Error && caught.message === 'no changes provided') {
        setMessage('Nothing changed');
      } else {
        setError(caught instanceof Error ? caught.message : 'Unable to save event');
      }
    } finally {
      setSaving(false);
    }
  }

  async function handlePublish() {
    if (!event || saving || actioning) return;
    if (dirty) {
      setError('Save your changes before publishing.');
      return;
    }
    setActioning(true);
    setMessage(null);
    setError(null);

    try {
      const published = await postJSON<EventDTO>(`/api/events/${event.id}/publish`, {});
      const publishedForm = formFromEvent(published);
      setEvent(published);
      setForm(publishedForm);
      setInitialForm(publishedForm);
      setMessage('Published');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to publish event');
    } finally {
      setActioning(false);
    }
  }

  async function handleEndOfNight() {
    if (!event || saving || actioning) return;
    if (dirty) {
      setError('Save your changes before ending the night.');
      return;
    }
    setActioning(true);
    setMessage(null);
    setError(null);

    try {
      const closedReport = await postJSON<EventReportDTO>(`/api/events/${event.id}/end-of-night`, {});
      setReport(closedReport);
      setEvent((current) => (current ? { ...current, status: 'end_of_night' } : current));
      setMessage('End of night complete');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to end event');
    } finally {
      setActioning(false);
    }
  }

  async function handleSettlementAdjustmentSubmit(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    if (!canManageSettlement) {
      setError('Finance access is required to change a settlement');
      return;
    }
    if (!event || !settlement || settlement.status !== 'open') {
      setError('Settlement is locked');
      return;
    }

    setSettlementSubmitting(true);
    setMessage(null);
    setError(null);

    try {
      const updatedSettlement = await postJSON<EventSettlementDTO>(`/api/events/${event.id}/settlement/adjustments`, {
        amountCents: priceInCents(settlementForm.amountDollars),
        label: settlementForm.label.trim(),
        reason: settlementForm.reason.trim(),
      });

      setSettlement(updatedSettlement);
      setSettlementForm(emptySettlementAdjustmentForm());
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to add adjustment');
    } finally {
      setSettlementSubmitting(false);
    }
  }

  async function handleArchiveNoteSubmit(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    if (!event || !archive) {
      setError('Archive is unavailable');
      return;
    }

    const body = archiveNoteBody.trim();
    if (!body) {
      setError('note body is required');
      return;
    }

    setArchiveSubmitting(true);
    setMessage(null);
    setError(null);

    try {
      const updatedArchive = await postJSON<EventArchiveDTO>(`/api/events/${event.id}/archive/notes`, { body });
      setArchive(updatedArchive);
      setArchiveNoteBody('');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to add lesson');
    } finally {
      setArchiveSubmitting(false);
    }
  }

  async function handleReviewApplication(applicationID: string, status: EventRoleApplicationDTO['status']) {
    if (!event || !canReviewApplications) return;

    setReviewingApplicationId(applicationID);
    setMessage(null);
    setError(null);

    try {
      const updated = await patchJSON<EventRoleApplicationDTO>(`/api/events/${event.id}/role-applications/${applicationID}`, { status });
      setApplications((current) => current?.map((application) => (application.id === applicationID ? updated : application)) ?? current);
      setApplicationReviewDrafts((current) => ({ ...current, [applicationID]: updated.status }));
      setNotificationsRefreshTick((current) => current + 1);
      setMessage('Application updated');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to review application');
    } finally {
      setReviewingApplicationId(null);
    }
  }

  async function handleCreateStaffing(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    if (!event || !canManageStaffing) return;

    const title = staffingForm.title.trim();
    if (!title) {
      setError('title is required');
      return;
    }

    setStaffingActioningId('new');
    setMessage(null);
    setError(null);

    try {
      const startsAt = staffingForm.startsAt ? fromInputValue(staffingForm.startsAt) : null;
      const endsAt = staffingForm.endsAt ? fromInputValue(staffingForm.endsAt) : null;

      const created = await postJSON<EventStaffingItemDTO>(`/api/events/${event.id}/staffing`, buildCreateRunOfShowPayload(staffingForm, startsAt, endsAt));

      setStaffingItems((current) => sortRunOfShowItems([...(current ?? []), created]));
      setStaffingForm(emptyRunOfShowForm());
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to create staffing item');
    } finally {
      setStaffingActioningId(null);
    }
  }

  async function handleStaffingUpdate(staffingID: string, payload: Record<string, unknown>) {
    if (!event || !canManageStaffing) return;

    setStaffingActioningId(staffingID);
    setMessage(null);
    setError(null);

    try {
      const updated = await patchJSON<EventStaffingItemDTO>(`/api/events/${event.id}/staffing/${staffingID}`, payload);
      setStaffingItems((current) => sortRunOfShowItems((current ?? []).map((item) => (item.id === staffingID ? updated : item))));
      setNotificationsRefreshTick((current) => current + 1);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to update staffing item');
    } finally {
      setStaffingActioningId(null);
    }
  }

  async function handleAssignStaffing(staffingID: string, formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    if (!canManageStaffing) return;

    const formData = new FormData(formEvent.currentTarget);
    const selection = String(formData.get('assignee') ?? '');

    if (!selection) {
      setError('Select an assignee');
      return;
    }

    const [targetType, targetID] = selection.split(':', 2);
    if (!targetType || !targetID) {
      setError('Select an assignee');
      return;
    }

    if (targetType === 'member') {
      await handleStaffingUpdate(staffingID, { assignedPersonId: targetID });
      return;
    }

    if (targetType === 'participant') {
      await handleStaffingUpdate(staffingID, { assignedApplicationId: targetID });
      return;
    }

    setError('Select an assignee');
  }

  async function handleClearStaffingAssignee(staffingID: string) {
    await handleStaffingUpdate(staffingID, { clearAssignee: true });
  }

  async function handleSetStaffingStatus(staffingID: string, status: EventStaffingItemDTO['status']) {
    await handleStaffingUpdate(staffingID, { status });
  }

  async function handleCommitmentCreate(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    if (!event || !currentWorkspace || currentWorkspace.id !== event.workspaceId || currentWorkspace.role !== 'owner') {
      return;
    }

    const title = commitmentForm.title.trim();
    if (!title) {
      setError('Enter a commitment title before saving it.');
      return;
    }

    setCommitmentSubmitting(true);
    setError(null);

    try {
      const created = await postJSON<CommitmentDTO>(`/api/workspaces/${event.workspaceId}/commitments`, {
        title,
        description: commitmentForm.description.trim(),
        dueAt: toRfc3339DateTime(commitmentForm.dueAt),
        eventId: event.id,
      });
      commitmentsRevisionRef.current += 1;
      setCommitments((current) => sortCommitments([...(current ?? []), created]));
      setCommitmentForm(emptyCommitmentForm());
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to save commitment');
    } finally {
      setCommitmentSubmitting(false);
    }
  }

  async function handleCommitmentStatus(commitmentID: string, status: CommitmentDTO['status']) {
    if (!event || !currentWorkspace || currentWorkspace.id !== event.workspaceId || currentWorkspace.role !== 'owner') {
      return;
    }

    setCommitmentActioningId(commitmentID);
    setError(null);

    try {
      const updated = await patchJSON<CommitmentDTO>(`/api/workspaces/${event.workspaceId}/commitments/${commitmentID}`, { status });
      commitmentsRevisionRef.current += 1;
      setCommitments((current) => sortCommitments([...(current ?? []).filter((commitment) => commitment.id !== updated.id), updated]));
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to update commitment');
    } finally {
      setCommitmentActioningId((current) => (current === commitmentID ? null : current));
    }
  }

  async function handleApplyTemplate(formEvent: FormEvent<HTMLFormElement>) {
    formEvent.preventDefault();
    if (!event || !currentWorkspace || currentWorkspace.id !== event.workspaceId || currentWorkspace.role !== 'owner' || !isDraftEvent(event.status)) {
      return;
    }

    if (!templateSelectionId) {
      setError('Select a template before applying it.');
      return;
    }

    setTemplateApplying(true);
    setMessage(null);
    setError(null);

    try {
      const updated = await postJSON<EventDTO>(`/api/events/${event.id}/apply-template`, { templateId: templateSelectionId });
      const updatedForm = formFromEvent(updated);
      setEvent(updated);
      setForm(updatedForm);
      setInitialForm(updatedForm);
      setMessage('Template applied');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to apply template');
    } finally {
      setTemplateApplying(false);
    }
  }

  async function handleSaveTemplateFromEvent() {
    if (!event || !currentWorkspace || currentWorkspace.id !== event.workspaceId || currentWorkspace.role !== 'owner') {
      return;
    }

    setTemplateSavingFromEvent(true);
    setMessage(null);
    setError(null);

    try {
      const created = await postJSON<EventTemplateDTO>(`/api/workspaces/${event.workspaceId}/event-templates`, {
        name: `${form.title.trim() || event.title} template`,
        title: form.title.trim() || event.title,
        publicDescription: form.publicDescription.trim(),
        locationDisplay: form.locationDisplay.trim(),
        ticketAllocation: Number(form.ticketAllocation),
        pricingMode: form.pricingMode,
        ticketPriceCents: form.pricingMode === 'fixed' ? priceInCents(form.ticketPriceDollars) : 0,
        ticketCurrency: 'usd',
        privateNotes: '',
      });
      setTemplates((current) => sortTemplates([...(current ?? []), created]));
      setMessage(`Saved ${created.name} as a template`);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to save template from event');
    } finally {
      setTemplateSavingFromEvent(false);
    }
  }

  async function handleSeedNextDraft() {
    if (!event || !archive) {
      setError('Archive is unavailable');
      return;
    }

    setActioning(true);
    setMessage(null);
    setError(null);

    try {
      const seeded = await postJSON<EventDTO>(`/api/events/${event.id}/archive/seed-draft`, {});
      setArchive((current) => (current ? { ...current, seededEventId: seeded.id } : current));
      setMessage(`Seeded next draft: ${seeded.title}`);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to seed next draft');
    } finally {
      setActioning(false);
    }
  }

  async function handleFinalizeSettlement() {
    if (!event || !settlement || settlement.status !== 'open' || !canManageSettlement) return;

    setSettlementFinalizing(true);
    setMessage(null);
    setError(null);

    try {
      const finalizedSettlement = await postJSON<EventSettlementDTO>(`/api/events/${event.id}/settlement/finalize`, {});
      setSettlement(finalizedSettlement);
      setMessage('Settlement finalized');
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to finalize settlement');
    } finally {
      setSettlementFinalizing(false);
    }
  }

  async function handleSettlementExport() {
    if (!event || !settlement || !canExportSettlement) return;
    setSettlementExporting(true);
    setMessage(null);
    setError(null);
    try {
      await downloadSettlementExport(event.id);
    } catch (caught) {
      if (caught instanceof ApiError && caught.status === 403) { setSettlement(null); setSettlementAccessDenied(true); }
      setError(caught instanceof Error ? caught.message : 'Unable to download settlement CSV');
    } finally {
      setSettlementExporting(false);
    }
  }

  async function handleSettlementReportExport(kind: 'markdown' | 'print') {
    if (!event || !settlement || !canExportSettlement || settlementExporting) return;
    setSettlementExporting(true);
    setMessage(null);
    setError(null);
    try {
      await downloadSettlementReport(event.id, kind);
    } catch (caught) {
      if (caught instanceof ApiError && caught.status === 403) {
        setSettlement(null);
        setSettlementAccessDenied(true);
      }
      setError(caught instanceof Error ? caught.message : 'Unable to download settlement report');
    } finally {
      setSettlementExporting(false);
    }
  }

  const effective = event ?? null;
  const lifecycleLabel = effective ? statusLabel(effective.status) : creating ? 'Draft' : 'Loading';
  const lifecycleTone = effective ? statusTone(effective.status) : 'border-white/10 bg-white/5 text-zinc-300';
  const lifecycleSummary = effective
    ? statusSummary(effective.status)
    : creating
      ? hasWorkspace
        ? 'Complete the form below to draft the event before publishing.'
        : 'Events are created from a workspace. Open one to start a new event.'
      : 'Loading event details.';

  return (
    <main className="min-h-screen px-4 py-6 text-zinc-100 sm:px-6 lg:px-8">
      <section className="mx-auto w-full max-w-4xl space-y-6">
        <header className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6 shadow-2xl shadow-black/40 backdrop-blur">
          <div className="flex flex-wrap items-start justify-between gap-4">
            <div>
              <p className="text-xs uppercase tracking-[0.3em] text-fuchsia-300">Event editor</p>
              <h1 className="mt-2 text-3xl font-semibold tracking-tight text-white">{creating ? 'New event' : effective?.title ?? 'Loading event'}</h1>
              <p className="mt-2 text-sm leading-6 text-zinc-400">Set the public page, ticket pricing, and door flow from one mobile-friendly editor.</p>
            </div>

            <div className="flex flex-wrap items-center gap-2 text-sm">
              <span className={`rounded-full border px-4 py-2 text-xs uppercase tracking-[0.25em] ${lifecycleTone}`}>{lifecycleLabel}</span>
              {effective?.publicUrl ? (
                <a className="rounded-full border border-white/10 bg-white/5 px-4 py-2 text-zinc-200 transition hover:bg-white/10" href={effective.publicUrl}>
                  Public page
                </a>
              ) : null}
              {effective ? (
                <a className="rounded-full border border-white/10 bg-white/5 px-4 py-2 text-zinc-200 transition hover:bg-white/10" href={`/door/${effective.id}`}>
                  Door
                </a>
              ) : null}
              <a className="rounded-full border border-white/10 bg-white/5 px-4 py-2 text-zinc-200 transition hover:bg-white/10" href="/workspace">
                Workspace
              </a>
            </div>
          </div>

          <p className="mt-4 max-w-2xl text-sm leading-6 text-zinc-400">{lifecycleSummary}</p>

          {effective ? (
            <div className="mt-6 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
              <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Lifecycle</p>
                <p className="mt-2 text-sm font-medium text-white">{lifecycleLabel}</p>
              </div>
              <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Public URL</p>
                {effective.publicUrl ? (
                  <a className="mt-2 block break-all text-sm font-medium text-white transition hover:text-amber-200" href={effective.publicUrl}>
                    {effective.publicUrl}
                  </a>
                ) : (
                  <p className="mt-2 text-sm font-medium text-white">Not published yet</p>
                )}
              </div>
              <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Door URL</p>
                <a className="mt-2 block break-all text-sm font-medium text-white transition hover:text-amber-200" href={`/door/${effective.id}`}>
                  /door/{effective.id}
                </a>
              </div>
              <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Reserved / checked in</p>
                <p className="mt-2 text-sm font-medium text-white">
                  {effective.reservedCount} / {effective.checkedInCount}
                </p>
              </div>
              <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Pricing</p>
                <p className="mt-2 text-sm font-medium text-white">{pricingSummary(effective)}</p>
              </div>
            </div>
          ) : null}
        </header>

        {loading ? <div className="rounded-[1.75rem] border border-white/10 bg-white/5 p-6 text-sm text-zinc-400">Loading event…</div> : null}
        {error ? <p className="rounded-2xl border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">{error}</p> : null}
        {workspaceError ? <div role="alert" className="rounded-2xl border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
          <p>Workspace access could not be loaded: {workspaceError}</p>
          <button type="button" className="mt-3 rounded-full border border-white/20 px-4 py-2" onClick={() => setWorkspaceRefreshTick((value) => value + 1)}>Retry workspace access</button>
        </div> : null}
        {reportError ? <div role="alert" className="rounded-2xl border border-rose-500/30 bg-rose-500/10 px-4 py-3 text-sm text-rose-200">
          <p>Report could not be loaded: {reportError}</p>
          <button type="button" className="mt-3 rounded-full border border-white/20 px-4 py-2" onClick={() => setReportRefreshTick((value) => value + 1)}>Retry report</button>
        </div> : null}
        {financeAccessDenied ? <div role="alert" className="rounded-2xl border border-amber-400/30 bg-amber-400/10 px-4 py-3 text-sm text-amber-100">
          Financial closeout details require an owner or finance workspace role.
        </div> : null}
        {message ? <p className="rounded-2xl border border-emerald-500/30 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-200">{message}</p> : null}

        {!loading ? (
          <div className="grid gap-6 lg:grid-cols-[1.15fr_0.85fr]">
            {creating && !hasWorkspace ? (
              <section className="space-y-4 rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Create from workspace</p>
                <h2 className="text-2xl font-semibold text-white">Events start inside a workspace</h2>
                <p className="max-w-xl text-sm leading-6 text-zinc-400">
                  Open the workspace first, then use its New event button so this event can inherit the right workspace context.
                </p>
                <div className="flex flex-wrap gap-3 text-sm">
                  <a className="rounded-2xl bg-white px-4 py-3 font-medium text-zinc-950 transition hover:bg-zinc-200" href="/workspace">
                    Go to workspace
                  </a>
                  <a className="rounded-2xl border border-white/10 bg-white/5 px-4 py-3 font-medium text-zinc-100 transition hover:bg-white/10" href="/">
                    Home
                  </a>
                </div>
              </section>
            ) : (
              <form className="space-y-4 rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6" onSubmit={handleSubmit}>
                <fieldset className="min-w-0 space-y-4 border-0 p-0" disabled={saving || actioning}>
                <legend className="sr-only">Event details</legend>
                <label className="block space-y-2 text-sm">
                  <span className="text-zinc-300">Title</span>
                  <input
                    className="w-full rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-white/8 disabled:cursor-not-allowed disabled:opacity-60"
                    value={form.title}
                    onChange={(event) => setForm((current) => ({ ...current, title: event.target.value }))}
                    required
                    disabled={closed}
                  />
                </label>

                <label className="block space-y-2 text-sm">
                  <span className="text-zinc-300">Starts at</span>
                  <input
                    className="w-full rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-white/8 disabled:cursor-not-allowed disabled:opacity-60"
                    type="datetime-local"
                    value={form.startsAt}
                    onChange={(event) => setForm((current) => ({ ...current, startsAt: event.target.value }))}
                    required
                    disabled={closed}
                  />
                </label>

                <label className="block space-y-2 text-sm">
                  <span className="text-zinc-300">Location</span>
                  <input
                    className="w-full rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-white/8 disabled:cursor-not-allowed disabled:opacity-60"
                    value={form.locationDisplay}
                    onChange={(event) => setForm((current) => ({ ...current, locationDisplay: event.target.value }))}
                    required
                    disabled={closed}
                  />
                </label>

                <label className="block space-y-2 text-sm">
                  <span className="text-zinc-300">Public description</span>
                  <textarea
                    className="min-h-40 w-full rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-white/8 disabled:cursor-not-allowed disabled:opacity-60"
                    value={form.publicDescription}
                    onChange={(event) => setForm((current) => ({ ...current, publicDescription: event.target.value }))}
                    required
                    disabled={closed}
                  />
                </label>

                <label className="block space-y-2 text-sm">
                  <span className="text-zinc-300">Ticket allocation</span>
                  <input
                    className="w-full rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-white/8 disabled:cursor-not-allowed disabled:opacity-60"
                    type="number"
                    min="1"
                    step="1"
                    value={form.ticketAllocation}
                    onChange={(event) => setForm((current) => ({ ...current, ticketAllocation: event.target.value }))}
                    required
                    disabled={closed}
                  />
                </label>

                <fieldset className={`rounded-[1.5rem] border p-4 ${pricingLocked || (!creating && !canEditPricing) ? 'border-white/10 bg-white/5 opacity-70' : 'border-white/10 bg-white/5'}`} disabled={pricingLocked || (!creating && !canEditPricing)}>
                  <div className="flex flex-wrap items-start justify-between gap-3">
                    <div>
                      <p className="text-xs uppercase tracking-[0.3em] text-fuchsia-300">Pricing</p>
                      <h2 className="mt-2 text-lg font-semibold text-white">Free or fixed paid tickets</h2>
                    </div>
                    <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-300">
                      USD only
                    </span>
                  </div>

                  <div className="mt-4 grid gap-3 sm:grid-cols-2">
                    <label className={`cursor-pointer rounded-2xl border p-4 transition ${form.pricingMode === 'free' ? 'border-amber-300/40 bg-amber-300/10 text-white' : 'border-white/10 bg-white/5 text-zinc-300 hover:bg-white/8'}`}>
                      <input
                        className="sr-only"
                        type="radio"
                        name="pricingMode"
                        value="free"
                        checked={form.pricingMode === 'free'}
                        onChange={() => setForm((current) => ({ ...current, pricingMode: 'free', ticketPriceDollars: '0.00' }))}
                        disabled={closed || pricingLocked || !canSelectFreePricing}
                      />
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <p className="text-sm font-semibold">Free reservation</p>
                          <p className="mt-1 text-sm leading-6 text-current/70">Guests reserve without paying. Keep the old no-cost flow.</p>
                        </div>
                        <span className="rounded-full border border-current/15 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em]">Free</span>
                      </div>
                    </label>

                    <label className={`cursor-pointer rounded-2xl border p-4 transition ${form.pricingMode === 'fixed' ? 'border-amber-300/40 bg-amber-300/10 text-white' : 'border-white/10 bg-white/5 text-zinc-300 hover:bg-white/8'}`}>
                      <input
                        className="sr-only"
                        type="radio"
                        name="pricingMode"
                        value="fixed"
                        checked={form.pricingMode === 'fixed'}
                        onChange={() => setForm((current) => ({ ...current, pricingMode: 'fixed' }))}
                        disabled={closed || pricingLocked || !canEditPricing}
                      />
                      <div className="flex items-start justify-between gap-3">
                        <div>
                          <p className="text-sm font-semibold">Fixed paid ticket</p>
                          <p className="mt-1 text-sm leading-6 text-current/70">Guests pay through Stripe Checkout in USD.</p>
                        </div>
                        <span className="rounded-full border border-current/15 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em]">Paid</span>
                      </div>
                    </label>
                  </div>

                  {form.pricingMode === 'fixed' ? (
                    <label className="mt-4 block space-y-2 text-sm">
                      <span className="text-zinc-300">Price in USD</span>
                      <input
                        className="w-full rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-white outline-none transition focus:border-amber-300/60 focus:bg-white/8 disabled:cursor-not-allowed disabled:opacity-60"
                        type="number"
                        min="0.5"
                        step="0.01"
                        inputMode="decimal"
                        value={form.ticketPriceDollars}
                        onChange={(event) => setForm((current) => ({ ...current, ticketPriceDollars: event.target.value }))}
                        required
                        disabled={closed || pricingLocked || !canEditPricing}
                      />
                      <p className="text-xs leading-5 text-zinc-500">Enter dollars; we convert to cents for checkout. Minimum recommended price is $0.50.</p>
                    </label>
                  ) : (
                    <p className="mt-4 text-sm leading-6 text-zinc-400">Free events keep the existing reservation flow and do not send guests to Stripe.</p>
                  )}

                  {pricingLocked ? <p className="mt-4 text-sm leading-6 text-zinc-400">Pricing is locked once tickets exist or after the event closes.</p> : null}
                  {!canEditPricing ? <p className="mt-4 text-sm leading-6 text-zinc-400">Fixed paid pricing requires an owner or finance workspace role. Free events can still be created.</p> : null}
                </fieldset>

                {!closed ? (
                  <button className="w-full rounded-2xl bg-amber-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-amber-200 disabled:cursor-not-allowed disabled:bg-amber-300/60" type="submit" disabled={saving || actioning || !dirty}>
                    {saving ? 'Saving…' : dirty ? (creating ? 'Create event' : 'Save event') : creating ? 'Fill in details' : 'No changes'}
                  </button>
                ) : (
                  <p className="rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-sm text-zinc-400">This event is closed. Editing is disabled.</p>
                )}
                </fieldset>
              </form>
            )}

            <aside className="space-y-6">
              {creating && hasWorkspace ? (
                <section className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                  <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Publish checklist</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">Ready to go live?</h2>
                  <ul className="mt-4 space-y-3 text-sm leading-6 text-zinc-400">
                    <li>• Title, start time, location, and public description are filled out.</li>
                    <li>• Ticket allocation matches the number of tickets you want to reserve.</li>
                    <li>• Save before publishing so the public page and Door links stay in sync.</li>
                  </ul>
                </section>
              ) : null}

              {!creating && isDraftEvent(effective?.status) ? (
                <section className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                  <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Publish checklist</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">Before you publish</h2>
                  <ul className="mt-4 space-y-3 text-sm leading-6 text-zinc-400">
                    <li>• Confirm the public title and description read well on mobile.</li>
                    <li>• Check the start time, location, and ticket allocation.</li>
                    <li>• Make sure the event is saved before you open the public page.</li>
                  </ul>
                </section>
              ) : null}

              {!creating && effective && currentWorkspace?.id === effective.workspaceId && currentWorkspace?.role === 'owner' ? (
                <section className="rounded-[1.75rem] border border-violet-400/20 bg-zinc-950/95 p-6 shadow-2xl shadow-black/30">
                  <p className="text-xs uppercase tracking-[0.3em] text-violet-300">Event templates</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">Private template tools</h2>
                  <p className="mt-2 text-sm leading-6 text-zinc-400">Template notes stay inside this private editor panel. Save the current event as a template or apply a saved one while the event is still a draft.</p>

                  {isDraftEvent(effective.status) ? (
                    <>
                      <form className="mt-4 space-y-4 rounded-2xl border border-white/10 bg-white/5 p-4" onSubmit={handleApplyTemplate}>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Template</span>
                          <select
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-violet-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                            value={templateSelectionId}
                            onChange={(selectEvent) => setTemplateSelectionId(selectEvent.target.value)}
                            disabled={templateApplying}
                          >
                            <option value="">Select a template</option>
                            {(templates ?? []).map((template) => (
                              <option key={template.id} value={template.id}>
                                {template.name} · {template.title}
                              </option>
                            ))}
                          </select>
                        </label>

                        <button className="rounded-2xl bg-violet-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-violet-200 disabled:cursor-not-allowed disabled:bg-violet-300/60" type="submit" disabled={templateApplying || !templateSelectionId}>
                          {templateApplying ? 'Applying…' : 'Apply template'}
                        </button>
                      </form>

                      {templates === null ? (
                        <p className="mt-4 rounded-2xl border border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">Loading templates…</p>
                      ) : templates.length === 0 ? (
                        <p className="mt-4 rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">No saved templates yet.</p>
                      ) : (
                        <div className="mt-4 space-y-3">
                          {templates.map((template) => (
                            <article key={template.id} className="rounded-2xl border border-white/10 bg-white/[0.03] p-4">
                              <div className="flex flex-wrap items-start justify-between gap-3">
                                <div>
                                  <p className="text-sm font-semibold text-white">{template.name}</p>
                                  <p className="mt-1 text-sm text-zinc-400">{template.title}</p>
                                </div>
                                <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-200">
                                  {template.pricingMode === 'free' ? 'Free' : `${template.ticketPriceCents / 100} USD`}
                                </span>
                              </div>

                              <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                                <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">{template.locationDisplay || 'No location set'}</span>
                                <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">{template.ticketAllocation} tickets</span>
                              </div>

                              <p className="mt-3 text-sm leading-6 text-zinc-300">
                                <span className="text-zinc-500">Private note:</span> {template.privateNotes || 'No private note yet.'}
                              </p>
                            </article>
                          ))}
                        </div>
                      )}
                    </>
                  ) : null}

                  <button
                    className="mt-4 rounded-2xl bg-violet-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-violet-200 disabled:cursor-not-allowed disabled:bg-violet-300/60"
                    type="button"
                    onClick={() => void handleSaveTemplateFromEvent()}
                    disabled={templateSavingFromEvent}
                  >
                    {templateSavingFromEvent ? 'Saving…' : 'Save as template'}
                  </button>
                </section>
              ) : null}

              {!creating && isPublishedEvent(effective?.status) ? (
                <section className="rounded-[1.75rem] border border-emerald-400/20 bg-emerald-400/10 p-6">
                  <p className="text-xs uppercase tracking-[0.3em] text-emerald-200">Live event</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">Next step: end of night</h2>
                  <div className="mt-4 space-y-3 text-sm">
                    {effective.publicUrl ? (
                      <a className="block rounded-2xl border border-white/10 bg-black/20 px-4 py-3 text-white transition hover:bg-black/30" href={effective.publicUrl}>
                        Public page: {effective.publicUrl}
                      </a>
                    ) : null}
                    <a className="block rounded-2xl border border-white/10 bg-black/20 px-4 py-3 text-white transition hover:bg-black/30" href={`/door/${effective.id}`}>
                      Door URL: /door/{effective.id}
                    </a>
                    <div className="rounded-2xl border border-white/10 bg-black/20 px-4 py-3 text-white">
                      Reserved {effective.reservedCount} · Checked in {effective.checkedInCount}
                    </div>
                    {dirty ? <p className="text-sm text-amber-200">Save your changes before ending the night.</p> : null}
                    <button className="door-action w-full rounded-2xl bg-white px-4 py-3 text-left font-medium text-zinc-950 transition hover:bg-zinc-200 disabled:cursor-not-allowed disabled:bg-white/70" type="button" onClick={handleEndOfNight} disabled={actioning || saving || dirty}>
                      End of night
                    </button>
                  </div>
                </section>
              ) : null}

              {report ? (
                <section className="rounded-[1.75rem] border border-fuchsia-400/20 bg-zinc-950/95 p-6 shadow-2xl shadow-black/30">
                  <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Report summary</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">{report.title}</h2>
                  <p className="mt-2 text-sm text-zinc-400">Generated {formatDateTime(report.generatedAt)} by {report.generatedByMemberEmail}</p>
                  <p className="mt-3 text-sm leading-6 text-zinc-300">{reportEndOfNightCopy()}</p>
                  <div className="mt-4 grid gap-3 sm:grid-cols-2">
                    <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                      <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Reserved</p>
                      <p className="mt-2 text-lg font-semibold text-white">{report.ticketsReserved}</p>
                    </div>
                    <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                      <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Checked in</p>
                      <p className="mt-2 text-lg font-semibold text-white">{report.ticketsCheckedIn}</p>
                    </div>
                    <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                      <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">No-shows</p>
                      <p className="mt-2 text-lg font-semibold text-white">{report.noShows}</p>
                    </div>
                    <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                      <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Allocation</p>
                      <p className="mt-2 text-lg font-semibold text-white">{report.ticketAllocation}</p>
                    </div>
                  </div>
                  {report.settlementSummary?.currency ? (
                    <div className="mt-4 rounded-2xl border border-white/10 bg-white/5 p-4">
                      <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Settlement summary</p>
                      <div className="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
                        <div>
                          <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Gross paid revenue</p>
                          <p className="mt-2 text-lg font-semibold text-white">
                            {formatMoney(report.settlementSummary.grossPaidRevenueCents, report.settlementSummary.currency)}
                          </p>
                        </div>
                        <div>
                          <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Paid tickets</p>
                          <p className="mt-2 text-lg font-semibold text-white">{report.settlementSummary.paidTicketCount}</p>
                        </div>
                        <div>
                          <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Pending tickets</p>
                          <p className="mt-2 text-lg font-semibold text-white">{report.settlementSummary.pendingTicketCount}</p>
                        </div>
                        <div>
                          <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Cancelled tickets</p>
                          <p className="mt-2 text-lg font-semibold text-white">{report.settlementSummary.cancelledTicketCount}</p>
                        </div>
                        <div>
                          <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Free tickets</p>
                          <p className="mt-2 text-lg font-semibold text-white">{report.settlementSummary.freeTicketCount}</p>
                        </div>
                        <div>
                          <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Reserved total</p>
                          <p className="mt-2 text-lg font-semibold text-white">{report.settlementSummary.reservedCount}</p>
                        </div>
                      </div>
                    </div>
                  ) : null}
                  <div className="mt-4 flex flex-wrap gap-3 text-sm">
                    <a className="rounded-2xl bg-white px-4 py-3 font-medium text-zinc-950 transition hover:bg-zinc-200" href={report.publicUrl}>
                      Public page
                    </a>
                    <a className="rounded-2xl border border-white/10 bg-white/5 px-4 py-3 font-medium text-zinc-100 transition hover:bg-white/10" href="/workspace">
                      Workspace
                    </a>
                  </div>
                </section>
              ) : null}

              {settlement ? (
                <section className="rounded-[1.75rem] border border-cyan-400/20 bg-zinc-950/95 p-6 shadow-2xl shadow-black/30">
                  <p className="text-xs uppercase tracking-[0.3em] text-cyan-300">Settlement closeout</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">Review adjustments</h2>
                  <p className="mt-2 text-sm text-zinc-400">Status: {settlementStatusLabel(settlementFinalized ? 'finalized' : 'open')}</p>

                  {canExportSettlement ? (
                    <div className="mt-4 flex flex-wrap gap-3">
                      <button
                        className="rounded-2xl border border-cyan-300/30 bg-cyan-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-cyan-200 disabled:cursor-not-allowed disabled:bg-cyan-300/60"
                        type="button"
                        onClick={handleSettlementExport}
                        disabled={settlementExporting}
                      >
                        {settlementExporting ? 'Preparing export…' : 'Download settlement CSV'}
                      </button>
                      <button
                        className="rounded-2xl border border-cyan-300/30 px-4 py-3 disabled:opacity-60"
                        type="button"
                        disabled={settlementExporting}
                        onClick={() => void handleSettlementReportExport('markdown')}
                      >
                        Download Markdown report
                      </button>
                      <button
                        className="rounded-2xl border border-cyan-300/30 px-4 py-3 disabled:opacity-60"
                        type="button"
                        disabled={settlementExporting}
                        onClick={() => void handleSettlementReportExport('print')}
                      >
                        Download printable HTML
                      </button>
                    </div>
                  ) : null}

                  {settlementFinalized ? (
                    <div className="mt-4 rounded-2xl border border-emerald-400/20 bg-emerald-400/10 p-4 text-sm text-emerald-100">
                      <p className="font-medium">Settlement locked</p>
                      <p className="mt-2 leading-6">{settlementLockedCopy(settlement.finalizedAt, settlement.finalizedByPersonId)}</p>
                    </div>
                  ) : null}

                  <div className="mt-4 grid gap-3 sm:grid-cols-3">
                    <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                      <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Gross revenue</p>
                      <p className="mt-2 text-lg font-semibold text-white">{formatMoney(settlement.grossPaidRevenueCents, settlement.currency)}</p>
                    </div>
                    <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                      <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Adjustment total</p>
                      <p className="mt-2 text-lg font-semibold text-white">{formatSignedMoney(settlement.adjustmentTotalCents, settlement.currency)}</p>
                    </div>
                    <div className="rounded-2xl border border-white/10 bg-white/5 p-4">
                      <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Net total</p>
                      <p className="mt-2 text-lg font-semibold text-white">{formatMoney(settlement.netTotalCents, settlement.currency)}</p>
                    </div>
                  </div>

                  <div className="mt-4 rounded-2xl border border-white/10 bg-white/5 p-4">
                    <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Adjustments</p>
                    {settlement.adjustments.length > 0 ? (
                      <div className="mt-3 space-y-3">
                        {settlement.adjustments.map((adjustment) => (
                          <div key={adjustment.id} className="rounded-2xl border border-white/10 bg-black/20 p-4">
                            <div className="flex flex-wrap items-start justify-between gap-3">
                              <div>
                                <p className="text-sm font-semibold text-white">{adjustment.label}</p>
                                <p className="mt-1 text-sm leading-6 text-zinc-400">{adjustment.reason}</p>
                              </div>
                              <p className="text-sm font-semibold text-white">{formatSignedMoney(adjustment.amountCents, settlement.currency)}</p>
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <p className="mt-3 text-sm leading-6 text-zinc-400">{settlementAdjustmentsEmptyCopy()}</p>
                    )}
                  </div>

                  <div className="mt-4 flex flex-wrap gap-3 text-sm">
                    {settlementOpen && canManageSettlement ? (
                      <button
                        className="rounded-2xl border border-emerald-400/20 bg-emerald-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-emerald-200 disabled:cursor-not-allowed disabled:bg-emerald-300/60"
                        type="button"
                        onClick={handleFinalizeSettlement}
                        disabled={settlementFinalizing}
                      >
                        {settlementFinalizing ? 'Finalizing…' : 'Finalize settlement'}
                      </button>
                    ) : null}
                  </div>

                  {settlementOpen && canManageSettlement ? (
                    <form className="mt-4 space-y-4 rounded-2xl border border-white/10 bg-white/5 p-4" onSubmit={handleSettlementAdjustmentSubmit}>
                      <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Add adjustment</p>
                      <label className="block space-y-2 text-sm">
                        <span className="text-zinc-300">Amount in USD</span>
                        <input
                          className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-cyan-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                          type="number"
                          step="0.01"
                          inputMode="decimal"
                          value={settlementForm.amountDollars}
                          onChange={(event) => setSettlementForm((current) => ({ ...current, amountDollars: event.target.value }))}
                          placeholder="-2.00"
                          required
                          disabled={settlementSubmitting}
                        />
                      </label>
                      <label className="block space-y-2 text-sm">
                        <span className="text-zinc-300">Label</span>
                        <input
                          className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-cyan-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                          value={settlementForm.label}
                          onChange={(event) => setSettlementForm((current) => ({ ...current, label: event.target.value }))}
                          required
                          disabled={settlementSubmitting}
                        />
                      </label>
                      <label className="block space-y-2 text-sm">
                        <span className="text-zinc-300">Reason</span>
                        <textarea
                          className="min-h-28 w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-cyan-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                          value={settlementForm.reason}
                          onChange={(event) => setSettlementForm((current) => ({ ...current, reason: event.target.value }))}
                          required
                          disabled={settlementSubmitting}
                        />
                      </label>
                      <button className="rounded-2xl bg-cyan-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-cyan-200 disabled:cursor-not-allowed disabled:bg-cyan-300/60" type="submit" disabled={settlementSubmitting}>
                        {settlementSubmitting ? 'Saving…' : 'Add adjustment'}
                      </button>
                    </form>
                  ) : !settlementOpen ? (
                    <p className="mt-4 rounded-2xl border border-white/10 bg-white/5 p-4 text-sm leading-6 text-zinc-400">{settlementAdjustmentsLockedCopy()}</p>
                  ) : null}
                </section>
              ) : null}

              {applicationsReady && event ? (
                <section className="rounded-[1.75rem] border border-emerald-400/20 bg-zinc-950/95 p-6 shadow-2xl shadow-black/30">
                  <p className="text-xs uppercase tracking-[0.3em] text-emerald-300">Applications</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">Private review</h2>
                  <p className="mt-2 text-sm leading-6 text-zinc-400">Owners and members can read submitted role applications. Owners can move each application through review.</p>

                  {applications && applications.length > 0 ? (
                    <div className="mt-4 space-y-3">
                      {applications.map((application) => {
                        const selectedStatus = applicationReviewDrafts[application.id] ?? application.status;
                        const roleName = roleNameById.get(application.roleId) ?? application.roleId;

                        return (
                          <article key={application.id} className="rounded-2xl border border-white/10 bg-white/5 p-4">
                            <div className="flex flex-wrap items-start justify-between gap-3">
                              <div>
                                <p className="text-sm font-semibold text-white">{application.applicantName}</p>
                                <p className="mt-1 text-sm text-zinc-400">{application.applicantEmail}</p>
                              </div>
                              <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-200">
                                {application.status}
                              </span>
                            </div>

                            <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                              <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Role {roleName}</span>
                              <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Created {formatDateTime(application.createdAt)}</span>
                              {application.reviewedAt ? <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Reviewed {formatDateTime(application.reviewedAt)}</span> : null}
                            </div>

                            <p className="mt-3 text-sm leading-6 text-zinc-300">{application.message || 'No message provided.'}</p>

                            {canReviewApplications ? (
                              <form
                                className="mt-4 flex flex-wrap items-end gap-3"
                                onSubmit={(submitEvent) => {
                                  submitEvent.preventDefault();
                                  void handleReviewApplication(application.id, selectedStatus);
                                }}
                              >
                                <label className="block min-w-44 space-y-2 text-sm">
                                  <span className="text-zinc-300">Status</span>
                                  <select
                                    className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-emerald-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                                    value={selectedStatus}
                                    onChange={(selectEvent) =>
                                      setApplicationReviewDrafts((current) => ({
                                        ...current,
                                        [application.id]: selectEvent.target.value as EventRoleApplicationDTO['status'],
                                      }))
                                    }
                                    disabled={reviewingApplicationId === application.id}
                                  >
                                    {applicationReviewStatusOptions.map((option) => (
                                      <option key={option.value} value={option.value}>
                                        {option.label}
                                      </option>
                                    ))}
                                  </select>
                                </label>

                                <button
                                  className="rounded-2xl bg-emerald-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-emerald-200 disabled:cursor-not-allowed disabled:bg-emerald-300/60"
                                  type="submit"
                                  disabled={reviewingApplicationId === application.id}
                                >
                                  {reviewingApplicationId === application.id ? 'Saving…' : 'Update status'}
                                </button>
                              </form>
                            ) : null}
                          </article>
                        );
                      })}
                    </div>
                  ) : (
                    <p className="mt-4 rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">No applications yet.</p>
                  )}
                </section>
              ) : null}

              {participantsReady && event ? (
                <section className="rounded-[1.75rem] border border-cyan-400/20 bg-zinc-950/95 p-6 shadow-2xl shadow-black/30">
                  <p className="text-xs uppercase tracking-[0.3em] text-cyan-300">Participant roster</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">Accepted participants</h2>
                  <p className="mt-2 text-sm leading-6 text-zinc-400">Accepted and confirmed applications stay visible here for the private operator team.</p>

                  {participants && participants.length > 0 ? (
                    <div className="mt-4 space-y-3">
                      {participants.map((participant) => (
                        <article key={participant.applicationId} className="rounded-2xl border border-white/10 bg-white/5 p-4">
                          <div className="flex flex-wrap items-start justify-between gap-3">
                            <div>
                              <p className="text-sm font-semibold text-white">{participant.applicantName}</p>
                              <p className="mt-1 text-sm text-zinc-400">{participant.applicantEmail}</p>
                            </div>
                            <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-200">
                              {participant.status}
                            </span>
                          </div>

                          <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                            <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Role {participant.roleName}</span>
                            <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Updated {formatDateTime(participant.updatedAt)}</span>
                          </div>
                        </article>
                      ))}
                    </div>
                  ) : (
                    <p className="mt-4 rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">No accepted participants yet.</p>
                  )}
                </section>
              ) : null}

              {staffingReady && event ? (
                <section className="rounded-[1.75rem] border border-sky-400/20 bg-zinc-950/95 p-6 shadow-2xl shadow-black/30">
                  <p className="text-xs uppercase tracking-[0.3em] text-sky-300">Staffing</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">Staffing board</h2>
                  <p className="mt-2 text-sm leading-6 text-zinc-400">Track tasks and shifts, then assign them to workspace members or accepted participants.</p>

                  <div className="mt-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
                    {([
                      ['open', 'Open'],
                      ['assigned', 'Assigned'],
                      ['completed', 'Completed'],
                      ['cancelled', 'Cancelled'],
                    ] as const).map(([status, label]) => (
                      <div key={status} className="rounded-2xl border border-white/10 bg-white/5 p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">{label}</p>
                        <p className="mt-2 text-lg font-semibold text-white">{staffingCounts[status]}</p>
                      </div>
                    ))}
                  </div>

                  {canManageStaffing ? (
                    <form className="mt-4 space-y-4 rounded-2xl border border-white/10 bg-white/5 p-4" onSubmit={handleCreateStaffing}>
                      <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Add staffing item</p>
                      <div className="grid gap-4 md:grid-cols-2">
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Title</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-sky-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                            value={staffingForm.title}
                            onChange={(event) => setStaffingForm((current) => ({ ...current, title: event.target.value }))}
                            required
                            disabled={staffingActioningId === 'new'}
                          />
                        </label>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Kind</span>
                          <select
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-sky-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                            value={staffingForm.kind}
                            onChange={(event) => setStaffingForm((current) => ({ ...current, kind: event.target.value as EventStaffingItemDTO['kind'] }))}
                            disabled={staffingActioningId === 'new'}
                          >
                            <option value="task">Task</option>
                            <option value="shift">Shift</option>
                          </select>
                        </label>
                      </div>

                      <label className="block space-y-2 text-sm">
                        <span className="text-zinc-300">Operator notes</span>
                        <textarea
                          className="min-h-28 w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-sky-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                          value={staffingForm.notes}
                          onChange={(event) => setStaffingForm((current) => ({ ...current, notes: event.target.value }))}
                          disabled={staffingActioningId === 'new'}
                        />
                      </label>

                      <label className="block space-y-2 text-sm">
                        <span className="text-zinc-300">Participant requirements</span>
                        <textarea
                          className="min-h-28 w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-sky-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                          value={staffingForm.participantRequirements}
                          onChange={(event) => setStaffingForm((current) => ({ ...current, participantRequirements: event.target.value }))}
                          disabled={staffingActioningId === 'new'}
                        />
                        <span className="block text-xs leading-5 text-zinc-500">Shared with the assigned person through their participant portal. Keep operator notes separate.</span>
                      </label>

                      <div className="grid gap-4 md:grid-cols-2">
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Starts at</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-sky-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                            type="datetime-local"
                            value={staffingForm.startsAt}
                            onChange={(event) => setStaffingForm((current) => ({ ...current, startsAt: event.target.value }))}
                            disabled={staffingActioningId === 'new'}
                          />
                        </label>
                        <label className="block space-y-2 text-sm">
                          <span className="text-zinc-300">Ends at</span>
                          <input
                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-sky-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                            type="datetime-local"
                            value={staffingForm.endsAt}
                            onChange={(event) => setStaffingForm((current) => ({ ...current, endsAt: event.target.value }))}
                            disabled={staffingActioningId === 'new'}
                          />
                        </label>
                      </div>

                      <button className="rounded-2xl bg-sky-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-sky-200 disabled:cursor-not-allowed disabled:bg-sky-300/60" type="submit" disabled={staffingActioningId === 'new'}>
                        {staffingActioningId === 'new' ? 'Saving…' : 'Add staffing item'}
                      </button>
                    </form>
                  ) : null}

                  {staffingLoading && !staffingItems ? <p className="mt-4 text-sm leading-6 text-zinc-400">Loading staffing board…</p> : null}

                  {!staffingLoading && staffingItems ? (
                    <div className="mt-4 space-y-4">
                      {staffingGroups.map(({ kind, label, items }) => (
                        <div key={kind} className="rounded-2xl border border-white/10 bg-white/5 p-4">
                          <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">{label}</p>

                          {items.length > 0 ? (
                            <div className="mt-3 space-y-3">
                              {items.map((item) => (
                                <article key={item.id} className="rounded-2xl border border-white/10 bg-zinc-950/50 p-4">
                                  <div className="flex flex-wrap items-start justify-between gap-3">
                                    <div>
                                      <p className="text-sm font-semibold text-white">{item.title}</p>
									  <p className="mt-1 text-sm leading-6 text-zinc-400">{item.notes || 'No operator notes yet.'}</p>
                                    </div>
                                    <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-200">
                                      {staffingStatusLabel(item.status)}
                                    </span>
                                  </div>

                                  <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                                    <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">{staffingKindLabel(item.kind)}</span>
                                    <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">{staffingWindowLabel(item)}</span>
                                    <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">{item.assigneeName ?? 'Unassigned'}</span>
                                  </div>

                                  {canManageStaffing ? (
                                    <div className="mt-4 space-y-3">
									  <form
										key={`${item.id}:${item.updatedAt}`}
										className="space-y-2"
										onSubmit={(submitEvent) => {
											submitEvent.preventDefault();
											const form = new FormData(submitEvent.currentTarget);
											void handleStaffingUpdate(item.id, { participantRequirements: String(form.get('participantRequirements') ?? '') });
										}}
									  >
										<label className="block space-y-2 text-sm">
										  <span className="text-zinc-300">Participant requirements</span>
										  <textarea name="participantRequirements" defaultValue={item.participantRequirements} className="min-h-24 w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-sky-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60" disabled={staffingActioningId === item.id} />
										</label>
										<p className="text-xs leading-5 text-zinc-500">Shared with the assigned person through their participant portal. It is never copied from operator notes.</p>
										<button className="rounded-2xl border border-white/10 bg-white/5 px-4 py-2 text-sm font-medium text-zinc-100 transition hover:bg-white/10 disabled:cursor-not-allowed disabled:bg-white/5" type="submit" disabled={staffingActioningId === item.id}>Save participant requirements</button>
									  </form>
                                      <form
                                        key={`${item.id}:${item.assignedPersonId ?? item.assignedApplicationId ?? 'none'}`}
                                        className="flex flex-wrap items-end gap-3"
                                        onSubmit={(submitEvent) => {
                                          void handleAssignStaffing(item.id, submitEvent);
                                        }}
                                      >
                                        <label className="block min-w-64 space-y-2 text-sm">
                                          <span className="text-zinc-300">Assign to</span>
                                          <select
                                            className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-sky-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                                            name="assignee"
                                            defaultValue={item.assignedPersonId ? `member:${item.assignedPersonId}` : item.assignedApplicationId ? `participant:${item.assignedApplicationId}` : ''}
                                            disabled={staffingActioningId === item.id}
                                          >
                                            <option value="">Select an assignee</option>
                                            <optgroup label="Workspace members">
                                              {staffingAssigneeOptions.members.map((option) => (
                                                <option key={option.value} value={option.value}>
                                                  {option.label}
                                                </option>
                                              ))}
                                            </optgroup>
                                            <optgroup label="Accepted participants">
                                              {staffingAssigneeOptions.participants.map((option) => (
                                                <option key={option.value} value={option.value}>
                                                  {option.label}
                                                </option>
                                              ))}
                                            </optgroup>
                                          </select>
                                        </label>

                                        <button className="rounded-2xl bg-sky-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-sky-200 disabled:cursor-not-allowed disabled:bg-sky-300/60" type="submit" disabled={staffingActioningId === item.id}>
                                          {staffingActioningId === item.id ? 'Saving…' : 'Assign'}
                                        </button>
                                      </form>

                                      <div className="flex flex-wrap gap-3 text-sm">
                                        <button
                                          className="rounded-2xl border border-white/10 bg-white/5 px-4 py-3 font-medium text-zinc-100 transition hover:bg-white/10 disabled:cursor-not-allowed disabled:bg-white/5"
                                          type="button"
                                          onClick={() => void handleClearStaffingAssignee(item.id)}
                                          disabled={staffingActioningId === item.id || (!item.assignedPersonId && !item.assignedApplicationId)}
                                        >
                                          Clear assignee
                                        </button>
                                        <button
                                          className="rounded-2xl border border-emerald-400/20 bg-emerald-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-emerald-200 disabled:cursor-not-allowed disabled:bg-emerald-300/60"
                                          type="button"
                                          onClick={() => void handleSetStaffingStatus(item.id, 'completed')}
                                          disabled={staffingActioningId === item.id || item.status === 'completed'}
                                        >
                                          Mark completed
                                        </button>
                                        <button
                                          className="rounded-2xl border border-rose-400/20 bg-rose-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-rose-200 disabled:cursor-not-allowed disabled:bg-rose-300/60"
                                          type="button"
                                          onClick={() => void handleSetStaffingStatus(item.id, 'cancelled')}
                                          disabled={staffingActioningId === item.id || item.status === 'cancelled'}
                                        >
                                          Mark cancelled
                                        </button>
                                      </div>
                                    </div>
                                  ) : null}
                                </article>
                              ))}
                            </div>
                          ) : (
                            <p className="mt-3 rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">No {label.toLowerCase()} yet.</p>
                          )}
                        </div>
                      ))}
                    </div>
                  ) : null}
                </section>
              ) : null}

              {canViewNotificationActivity && event && notifications !== null ? (
                <section className="rounded-[1.75rem] border border-amber-400/20 bg-zinc-950/95 p-6 shadow-2xl shadow-black/30">
                  <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Notification activity</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">
                    {loadedNotificationActivity === null ? 'Loading notifications…' : `${loadedNotificationActivity.length} queued notification${loadedNotificationActivity.length === 1 ? '' : 's'}`}
                  </h2>
                  <p className="mt-2 text-sm leading-6 text-zinc-400">Recent operator-visible notifications stay here without application messages, staffing notes, or full email bodies.</p>

                  {loadedNotificationActivity === null ? (
                    <p className="mt-4 text-sm leading-6 text-zinc-400">Loading notification activity…</p>
                  ) : loadedNotificationActivity.length > 0 ? (
                    <div className="mt-4 space-y-3">
                      {loadedNotificationActivity.map((notification) => (
                        <article key={notification.id} className="rounded-2xl border border-white/10 bg-white/5 p-4">
                          <div className="flex flex-wrap items-start justify-between gap-3">
                            <div>
                              <p className="text-sm font-semibold text-white">{notification.subject}</p>
                              <p className="mt-1 text-sm text-zinc-400">{notification.recipientEmail}</p>
                            </div>
                            <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-200">
                              {notification.status}
                            </span>
                          </div>

                          <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                            <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Type {notification.notificationType}</span>
                            <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Preview {notification.preview}</span>
                            <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Created {formatDateTime(notification.createdAt)}</span>
                          </div>
                        </article>
                      ))}
                    </div>
                  ) : (
                    <p className="mt-4 rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">No notification activity yet.</p>
                  )}
                </section>
              ) : null}

              {canViewReminderActivity && event && reminders !== null ? (
                <section className="rounded-[1.75rem] border border-fuchsia-400/20 bg-zinc-950/95 p-6 shadow-2xl shadow-black/30">
                  <p className="text-xs uppercase tracking-[0.3em] text-fuchsia-300">Reminder activity</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">
                    {loadedReminderActivity === null ? 'Loading reminders…' : `${loadedReminderActivity.length} queued reminder${loadedReminderActivity.length === 1 ? '' : 's'}`}
                  </h2>
                  <p className="mt-2 text-sm leading-6 text-zinc-400">Event reminder sweeps stay here without private commit text, staffing notes, or public copy.</p>

                  {loadedReminderActivity === null ? (
                    <p className="mt-4 text-sm leading-6 text-zinc-400">Loading reminder activity…</p>
                  ) : loadedReminderActivity.length > 0 ? (
                    <div className="mt-4 space-y-3">
                      {loadedReminderActivity.map((reminder) => (
                        <article key={reminder.id} className="rounded-2xl border border-white/10 bg-white/5 p-4">
                          <div className="flex flex-wrap items-start justify-between gap-3">
                            <div>
                              <p className="text-sm font-semibold text-white">{reminder.subject}</p>
                              <p className="mt-1 text-sm text-zinc-400">{reminder.recipientEmail}</p>
                            </div>
                            <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-200">
                              {reminder.status}
                            </span>
                          </div>

                          <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                            <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Type {reminder.reminderType}</span>
                            <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Preview {reminder.preview}</span>
                            <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Due {formatDateTime(reminder.dueAt)}</span>
                          </div>
                        </article>
                      ))}
                    </div>
                  ) : (
                    <p className="mt-4 rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">No reminder activity yet.</p>
                  )}
                </section>
              ) : null}

              {archiveLoading || archive ? (
                <section className="rounded-[1.75rem] border border-violet-400/20 bg-zinc-950/95 p-6 shadow-2xl shadow-black/30">
                  <p className="text-xs uppercase tracking-[0.3em] text-violet-300">Private archive</p>
                  <h2 className="mt-2 text-2xl font-semibold text-white">Lessons learned</h2>

                  {archiveLoading && !archive ? (
                    <p className="mt-4 text-sm leading-6 text-zinc-400">Loading private archive…</p>
                  ) : archive ? (
                    <>
                      <p className="mt-2 text-sm text-zinc-400">Status: private workspace memory</p>
                      <p className="mt-2 rounded-2xl border border-white/10 bg-white/5 p-4 text-sm leading-6 text-zinc-300">
                        Private notes stay in the archive. The next draft starts clean.
                      </p>
                      <p className="mt-3 rounded-2xl border border-violet-400/20 bg-violet-400/10 p-4 text-sm leading-6 text-violet-50">
                        {archive.noteCount === 0
                          ? 'Capture one lesson before seeding the next draft.'
                          : 'Use these notes while planning the next event.'}
                      </p>
                      <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                        <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Created {formatDateTime(archive.createdAt)}</span>
                        <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Updated {formatDateTime(archive.updatedAt)}</span>
                        <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Report {archive.reportId}</span>
                        <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">Settlement {archive.settlementId}</span>
                      </div>
                      <p className="mt-3 text-sm font-medium text-zinc-200">{archive.noteCount === 1 ? '1 note' : `${archive.noteCount} notes`}</p>

                      <div className="mt-4 space-y-3">
                        {archive.notes.length > 0 ? (
                          archive.notes.map((note) => (
                            <article key={note.id} className="rounded-2xl border border-white/10 bg-white/5 p-4">
                              <p className="text-sm leading-6 text-zinc-100">{note.body}</p>
                              <p className="mt-2 text-xs uppercase tracking-[0.2em] text-zinc-500">{formatDateTime(note.createdAt)}</p>
                            </article>
                          ))
                        ) : (
                          <p className="rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">No lessons yet.</p>
                        )}
                      </div>

                      <div className="mt-4 rounded-2xl border border-white/10 bg-white/5 p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Participant memory</p>
                        <p className="mt-2 text-sm leading-6 text-zinc-400">Accepted and confirmed participants are preserved here without private emails or messages.</p>
                        {archive.participants.length > 0 ? (
                          <div className="mt-3 space-y-3">
                            {archive.participants.map((participant) => (
                              <article key={participant.sourceApplicationId} className="rounded-2xl border border-white/10 bg-zinc-950/50 p-4">
                                <div className="flex flex-wrap items-start justify-between gap-3">
                                  <div>
                                    <p className="text-sm font-semibold text-white">{participant.participantName}</p>
                                    <p className="mt-1 text-sm text-zinc-400">Role {participant.roleName}</p>
                                  </div>
                                  <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-200">
                                    {participant.status}
                                  </span>
                                </div>
                              </article>
                            ))}
                          </div>
                        ) : (
                          <p className="mt-3 rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">No participant memory yet.</p>
                        )}
                      </div>

                      <div className="mt-4 rounded-2xl border border-white/10 bg-white/5 p-4">
                        <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Staffing memory</p>
                        <p className="mt-2 text-sm leading-6 text-zinc-400">Non-cancelled staffing items are preserved here without private notes.</p>
                        {archive.staffingItems.length > 0 ? (
                          <div className="mt-3 space-y-3">
                            {archive.staffingItems.map((item) => (
                              <article key={item.sourceStaffingItemId} className="rounded-2xl border border-white/10 bg-zinc-950/50 p-4">
                                <div className="flex flex-wrap items-start justify-between gap-3">
                                  <div>
                                    <p className="text-sm font-semibold text-white">{item.title}</p>
                                    <p className="mt-1 text-sm text-zinc-400">{staffingKindLabel(item.kind)}</p>
                                  </div>
                                  <span className="rounded-full border border-white/10 bg-black/20 px-3 py-1 text-[0.7rem] font-semibold uppercase tracking-[0.28em] text-zinc-200">
                                    {staffingStatusLabel(item.status)}
                                  </span>
                                </div>
                                <div className="mt-3 flex flex-wrap gap-2 text-[0.7rem] uppercase tracking-[0.2em] text-zinc-500">
                                  <span className="rounded-full border border-white/10 bg-white/5 px-3 py-1">{item.assigneeName ?? 'Unassigned'}</span>
                                </div>
                              </article>
                            ))}
                          </div>
                        ) : (
                          <p className="mt-3 rounded-2xl border border-dashed border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">No staffing memory yet.</p>
                        )}
                        {archive.staffingItems.some((item) => item.status !== 'completed') ? (
                          <p className="mt-3 rounded-2xl border border-violet-400/20 bg-violet-400/10 p-4 text-sm leading-6 text-violet-50">
                            Unresolved staffing should inform next draft planning.
                          </p>
                        ) : (
                          <p className="mt-3 rounded-2xl border border-white/10 bg-white/[0.03] p-4 text-sm leading-6 text-zinc-400">
                            Staffing memory is ready for the next draft.
                          </p>
                        )}
                      </div>

                      {canManageArchive ? (
                        <>
                          <div className="mt-4 rounded-2xl border border-white/10 bg-white/5 p-4">
                            <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Workspace archive</p>
                            <p className="mt-2 text-sm leading-6 text-zinc-400">Review closed-event notes, then jump back to the workspace archive or continue with the next draft.</p>
                            <div className="mt-3 flex flex-wrap gap-3 text-sm">
                              <a className="rounded-2xl border border-white/10 bg-white/5 px-4 py-3 font-medium text-zinc-100 transition hover:bg-white/10" href={`/workspace?workspaceId=${event.workspaceId}`}>
                                Back to workspace archive
                              </a>
                              <a className="rounded-2xl border border-violet-400/30 bg-violet-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-violet-200" href={`/events/${event.id}/public-archive`}>
                                Manage future public archive
                              </a>
                              {archive.seededEventId ? (
                                <a className="rounded-2xl border border-violet-400/30 bg-violet-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-violet-200" href={`/events/${archive.seededEventId}?workspaceId=${event.workspaceId}`}>
                                  Open seeded draft
                                </a>
                              ) : (
                                <button className="rounded-2xl border border-violet-400/30 bg-violet-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-violet-200 disabled:cursor-not-allowed disabled:bg-violet-300/60" type="button" onClick={handleSeedNextDraft} disabled={actioning}>
                                  {actioning ? 'Seeding…' : 'Seed next draft'}
                                </button>
                              )}
                            </div>
                          </div>

                          <form className="mt-4 space-y-4 rounded-2xl border border-white/10 bg-white/5 p-4" onSubmit={handleArchiveNoteSubmit}>
                            <p className="text-xs uppercase tracking-[0.2em] text-zinc-500">Add lesson</p>
                            <label className="block space-y-2 text-sm">
                              <span className="text-zinc-300">Write a note for the next closeout</span>
                              <textarea
                                className="min-h-28 w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-violet-300/60 focus:bg-zinc-950/80 disabled:cursor-not-allowed disabled:opacity-60"
                                value={archiveNoteBody}
                                onChange={(event) => setArchiveNoteBody(event.target.value)}
                                placeholder="Move doors earlier."
                                disabled={archiveSubmitting}
                              />
                            </label>
                            <button className="rounded-2xl bg-violet-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-violet-200 disabled:cursor-not-allowed disabled:bg-violet-300/60" type="submit" disabled={archiveSubmitting}>
                              {archiveSubmitting ? 'Saving…' : 'Add lesson'}
                            </button>
                          </form>
                        </>
                      ) : null}
                    </>
                  ) : null}
                </section>
              ) : null}

              {!creating && effective && !isClosedEvent(effective.status) ? (
                <section className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                  <p className="text-xs uppercase tracking-[0.3em] text-amber-300">Actions</p>
                  <div className="mt-4 flex flex-col gap-3">
                    {isDraftEvent(effective.status) && dirty ? <p className="text-sm text-amber-200">Save your changes before publishing.</p> : null}
                    {isDraftEvent(effective.status) ? (
                      <button className="door-action rounded-2xl bg-white px-4 py-3 text-left font-medium text-zinc-950 transition hover:bg-zinc-200 disabled:cursor-not-allowed disabled:bg-white/70" type="button" onClick={handlePublish} disabled={actioning || saving || dirty}>
                        Publish public page
                      </button>
                    ) : null}

                    {effective.publicUrl ? (
                      <a className="door-action rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-left font-medium text-zinc-100 transition hover:bg-white/10" href={effective.publicUrl}>
                        Open public URL
                      </a>
                    ) : null}

                    <a className="door-action rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-left font-medium text-zinc-100 transition hover:bg-white/10" href={`/door/${effective.id}`}>
                      Open Door
                    </a>
                  </div>
                </section>
              ) : null}

              {!creating && effective && currentWorkspace?.id === effective.workspaceId && !commitmentsDenied ? (
                commitments === null ? (
                  <section className="space-y-4 rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                    <div>
                      <p className="text-xs uppercase tracking-[0.3em] text-fuchsia-300">Event commitments</p>
                      <p className="mt-2 text-sm leading-6 text-zinc-400">Loading commitments…</p>
                    </div>
                  </section>
                ) : (
                <section className="space-y-4 rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                  <div>
                    <p className="text-xs uppercase tracking-[0.3em] text-fuchsia-300">Event commitments</p>
                    <p className="mt-2 text-sm leading-6 text-zinc-400">Private promises for {effective.title} stay tied to this workspace only.</p>
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
                      <p className="font-medium text-white">No commitments yet for this event.</p>
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
                                {commitment.ownerPersonId ? ' · Owner assigned' : ''}
                              </p>
                            </div>
                            <span className={`rounded-full border px-3 py-1 text-xs uppercase tracking-[0.25em] ${commitmentStatusTone(commitment.status)}`}>
                              {commitmentStatusLabel(commitment.status)}
                            </span>
                          </div>

                          <p className="mt-3 text-sm leading-6 text-zinc-300">{commitment.description || 'No private description yet.'}</p>

                          {currentWorkspace?.role === 'owner' ? (
                            <div className="mt-4 flex flex-wrap gap-2 text-sm">
                              <button
                                className="rounded-full border border-emerald-400/20 bg-emerald-300 px-3 py-2 font-medium text-zinc-950 transition hover:bg-emerald-200 disabled:cursor-not-allowed disabled:bg-emerald-300/60"
                                type="button"
                                onClick={() => void handleCommitmentStatus(commitment.id, 'done')}
                                disabled={commitmentActioningId === commitment.id}
                              >
                                Mark done
                              </button>
                              <button
                                className="rounded-full border border-white/10 bg-white/5 px-3 py-2 text-zinc-200 transition hover:bg-white/10 disabled:cursor-not-allowed disabled:bg-white/5"
                                type="button"
                                onClick={() => void handleCommitmentStatus(commitment.id, 'open')}
                                disabled={commitmentActioningId === commitment.id}
                              >
                                Reopen
                              </button>
                              <button
                                className="rounded-full border border-rose-400/20 bg-rose-300 px-3 py-2 font-medium text-zinc-950 transition hover:bg-rose-200 disabled:cursor-not-allowed disabled:bg-rose-300/60"
                                type="button"
                                onClick={() => void handleCommitmentStatus(commitment.id, 'cancelled')}
                                disabled={commitmentActioningId === commitment.id}
                              >
                                Cancel
                              </button>
                            </div>
                          ) : null}
                        </article>
                      ))}
                    </div>
                  )}

                  {currentWorkspace?.role === 'owner' ? (
                    <form className="space-y-4 rounded-2xl border border-white/10 bg-white/[0.03] p-4" onSubmit={handleCommitmentCreate}>
                      <p className="text-xs uppercase tracking-[0.3em] text-zinc-500">Add commitment</p>
                      <label className="block space-y-2 text-sm">
                        <span className="text-zinc-300">Title</span>
                        <input
                          className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-fuchsia-300/60 focus:bg-zinc-950/80"
                          value={commitmentForm.title}
                          onChange={(event) => setCommitmentForm((current) => ({ ...current, title: event.target.value }))}
                          required
                        />
                      </label>
                      <label className="block space-y-2 text-sm">
                        <span className="text-zinc-300">Description</span>
                        <textarea
                          className="min-h-28 w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-fuchsia-300/60 focus:bg-zinc-950/80"
                          value={commitmentForm.description}
                          onChange={(event) => setCommitmentForm((current) => ({ ...current, description: event.target.value }))}
                        />
                      </label>
                      <label className="block space-y-2 text-sm">
                        <span className="text-zinc-300">Due at</span>
                        <input
                          className="w-full rounded-2xl border border-white/10 bg-zinc-950/60 px-4 py-3 text-white outline-none transition focus:border-fuchsia-300/60 focus:bg-zinc-950/80"
                          type="datetime-local"
                          value={commitmentForm.dueAt}
                          onChange={(event) => setCommitmentForm((current) => ({ ...current, dueAt: event.target.value }))}
                        />
                      </label>
                      <button className="rounded-2xl bg-fuchsia-300 px-4 py-3 font-medium text-zinc-950 transition hover:bg-fuchsia-200 disabled:cursor-not-allowed disabled:bg-fuchsia-300/60" type="submit" disabled={commitmentSubmitting}>
                        {commitmentSubmitting ? 'Saving…' : 'Add commitment'}
                      </button>
                    </form>
                  ) : null}
                </section>
                )
              ) : null}

              {creating && hasWorkspace ? (
                <section className="rounded-[1.75rem] border border-white/10 bg-zinc-950/85 p-6">
                  <p className="text-xs uppercase tracking-[0.3em] text-zinc-500">Workspace link</p>
                  <p className="mt-2 text-sm leading-6 text-zinc-400">Finish the draft here, then return to the workspace to publish or share it.</p>
                  <a className="mt-4 inline-flex rounded-2xl border border-white/10 bg-white/5 px-4 py-3 text-sm font-medium text-zinc-100 transition hover:bg-white/10" href="/workspace">
                    Back to workspace
                  </a>
                </section>
              ) : null}
            </aside>
          </div>
        ) : null}
      </section>
    </main>
  );
}
