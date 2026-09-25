import { useLocalSearchParams } from 'expo-router';
import { CheckCircle2, ChevronLeft, Circle, Clock, Plus, X } from 'lucide-react-native';
import { useEffect, useState } from 'react';
import { Pressable, ScrollView, StyleSheet, Text, TextInput, View } from 'react-native';

import { formatTime } from '@/api/format';
import { createEventStaffing, listEventStaffing, updateEventStaffing, updateEventStaffingStatus } from '@/api/staff';
import type { EventStaffingItemDTO } from '@/api/types';
import { safeBack } from '@/navigation/safeBack';
import {
	applyRunOfShowStatusUpdate,
	buildCreateRunOfShowPayload,
	buildUpdateRunOfShowPayload,
	emptyRunOfShowForm,
	isRunOfShowTimeRangeValid,
	parseOptionalDateTime,
	sortRunOfShowItems,
	toRunOfShowInputValue,
} from '@/modules/runOfShow/runOfShowModel';

export default function RunOfShowScreen() {
  const params = useLocalSearchParams<{ eventId?: string }>();
  const eventID = typeof params.eventId === 'string' ? params.eventId : '';
  const [items, setItems] = useState<EventStaffingItemDTO[]>([]);
  const [loading, setLoading] = useState(Boolean(eventID));
  const [error, setError] = useState<string | null>(eventID ? null : 'Missing event ID. Open Run of Show from Staff mode.');
  const [updatingID, setUpdatingID] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [editingID, setEditingID] = useState<string | null>(null);
  const [form, setForm] = useState(emptyRunOfShowForm());

  useEffect(() => {
    let cancelled = false;
    async function load() {
      if (!eventID) return;
      setLoading(true);
      setError(null);
      try {
        const loaded = await listEventStaffing(eventID);
        if (!cancelled) {
          setItems(sortRunOfShowItems(loaded));
        }
      } catch (caught) {
        if (!cancelled) setError(caught instanceof Error ? caught.message : 'Unable to load run of show');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, [eventID]);

  async function updateStatus(item: EventStaffingItemDTO, status: EventStaffingItemDTO['status']) {
    if (!eventID || updatingID) return;
    setUpdatingID(item.id);
    setError(null);
    try {
      const updated = await updateEventStaffingStatus(eventID, item.id, status);
      setItems((current) => applyRunOfShowStatusUpdate(current, updated));
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to update staffing item');
    } finally {
      setUpdatingID(null);
    }
  }

  async function saveForm() {
    if (!eventID || creating) return;
    const title = form.title.trim();
    if (!title) {
      setError('Title is required for a run-of-show item.');
      return;
    }
    const startsAt = parseOptionalDateTime(form.startsAt);
    const endsAt = parseOptionalDateTime(form.endsAt);
    if (startsAt === false || endsAt === false) {
      setError('Use date/time like 2026-06-19 21:00, or leave it blank.');
      return;
    }
    if (!isRunOfShowTimeRangeValid(startsAt, endsAt)) {
      setError('End time must be after start time.');
      return;
    }
    setCreating(true);
    setError(null);
    try {
      if (editingID) {
		const updated = await updateEventStaffing(eventID, editingID, buildUpdateRunOfShowPayload({ title, notes: form.notes, participantRequirements: form.participantRequirements }, startsAt, endsAt));
        setItems((current) => sortRunOfShowItems(current.map((candidate) => (candidate.id === updated.id ? updated : candidate))));
        stopEditing();
      } else {
        const created = await createEventStaffing(eventID, buildCreateRunOfShowPayload(form, startsAt, endsAt));
        setItems((current) => sortRunOfShowItems([created, ...current]));
        setForm(emptyRunOfShowForm());
      }
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : 'Unable to save run-of-show item');
    } finally {
      setCreating(false);
    }
  }

  function startEditing(item: EventStaffingItemDTO) {
    setEditingID(item.id);
    setForm({
      title: item.title,
      kind: item.kind,
      notes: item.notes,
		participantRequirements: item.participantRequirements,
      startsAt: toRunOfShowInputValue(item.startsAt),
      endsAt: toRunOfShowInputValue(item.endsAt),
    });
    setError(null);
  }

  function stopEditing() {
    setEditingID(null);
    setForm(emptyRunOfShowForm());
  }

  return (
    <View style={styles.screen}>
      <View style={styles.header}>
        <Pressable onPress={() => safeBack('/staff')} style={styles.backButton}>
          <ChevronLeft size={24} color="#171717" />
        </Pressable>
        <Text style={styles.title}>Run of Show</Text>
      </View>
      <ScrollView style={styles.scroller} contentContainerStyle={styles.timeline}>
        <View style={styles.createPanel}>
          <View style={styles.panelHeaderRow}>
            <Text style={styles.panelTitle}>{editingID ? 'Edit run-of-show item' : 'Add run-of-show item'}</Text>
            {editingID ? (
              <Pressable onPress={stopEditing} style={styles.cancelEditButton}>
                <X size={16} color="#737373" />
              </Pressable>
            ) : null}
          </View>
          <TextInput
            value={form.title}
            onChangeText={(value) => setForm((current) => ({ ...current, title: value }))}
            placeholder="Task or shift title"
            placeholderTextColor="#a3a3a3"
            style={styles.input}
          />
		  <TextInput
			value={form.participantRequirements}
			onChangeText={(value) => setForm((current) => ({ ...current, participantRequirements: value }))}
			placeholder="Participant requirements (shared with the assigned person through their participant portal)"
			placeholderTextColor="#a3a3a3"
			multiline
			style={[styles.input, styles.notesInput]}
		  />
          <View style={styles.kindRow}>
            <KindButton label="Task" selected={form.kind === 'task'} disabled={Boolean(editingID)} onPress={() => setForm((current) => ({ ...current, kind: 'task' }))} />
            <KindButton label="Shift" selected={form.kind === 'shift'} disabled={Boolean(editingID)} onPress={() => setForm((current) => ({ ...current, kind: 'shift' }))} />
          </View>
          <View style={styles.timeInputsRow}>
            <TextInput
              value={form.startsAt}
              onChangeText={(value) => setForm((current) => ({ ...current, startsAt: value }))}
              placeholder="Start time"
              placeholderTextColor="#a3a3a3"
              style={[styles.input, styles.timeInput]}
            />
            <TextInput
              value={form.endsAt}
              onChangeText={(value) => setForm((current) => ({ ...current, endsAt: value }))}
              placeholder="End time"
              placeholderTextColor="#a3a3a3"
              style={[styles.input, styles.timeInput]}
            />
          </View>
          <TextInput
            value={form.notes}
            onChangeText={(value) => setForm((current) => ({ ...current, notes: value }))}
            placeholder="Notes"
            placeholderTextColor="#a3a3a3"
            multiline
            style={[styles.input, styles.notesInput]}
          />
          <Pressable disabled={creating} onPress={() => void saveForm()} style={[styles.createButton, creating && styles.createButtonDisabled]}>
            <Plus size={18} color="#ffffff" />
            <Text style={styles.createButtonText}>{creating ? 'Saving…' : editingID ? 'Save changes' : 'Add item'}</Text>
          </Pressable>
          <Text style={styles.helpText}>{editingID ? 'Kind cannot be changed after creation yet. Times are optional.' : 'Times are optional. Use local format like 2026-06-19 21:00.'}</Text>
        </View>
        {loading ? <Text style={styles.message}>Loading run of show…</Text> : null}
        {error ? <Text style={styles.error}>{error}</Text> : null}
        {!loading && !error && items.length === 0 ? <Text style={styles.message}>No staffing items yet.</Text> : null}
        {items.map((task) => (
          <View key={task.id} style={styles.taskRow}>
            <Text style={styles.time}>{task.startsAt ? formatTime(task.startsAt) : '—'}</Text>
            <View style={styles.iconWrap}>{statusIcon(task.status)}</View>
            <View style={[styles.taskBody, task.status === 'completed' && styles.completed]}>
              <Text style={styles.taskTitle}>{task.title}</Text>
              <Text style={styles.assignee}>{task.assigneeName ?? task.kind}</Text>
              {task.notes ? <Text style={styles.notes}>{task.notes}</Text> : null}
              <View style={styles.actionsRow}>
                {task.status !== 'completed' ? (
                  <Pressable disabled={updatingID === task.id} onPress={() => void updateStatus(task, 'completed')} style={styles.actionButton}>
                    <Text style={styles.actionButtonText}>{updatingID === task.id ? 'Saving…' : 'Complete'}</Text>
                  </Pressable>
                ) : (
                  <Pressable disabled={updatingID === task.id} onPress={() => void updateStatus(task, task.assignedPersonId || task.assignedApplicationId ? 'assigned' : 'open')} style={styles.actionButtonMuted}>
                    <Text style={styles.actionButtonMutedText}>Reopen</Text>
                  </Pressable>
                )}
                {task.status !== 'cancelled' ? (
                  <Pressable disabled={updatingID === task.id} onPress={() => void updateStatus(task, 'cancelled')} style={styles.actionButtonMuted}>
                    <Text style={styles.actionButtonMutedText}>Cancel</Text>
                  </Pressable>
                ) : null}
                <Pressable disabled={updatingID === task.id} onPress={() => startEditing(task)} style={styles.actionButtonMuted}>
                  <Text style={styles.actionButtonMutedText}>Edit</Text>
                </Pressable>
              </View>
            </View>
          </View>
        ))}
      </ScrollView>
    </View>
  );
}

function KindButton({ label, selected, disabled, onPress }: { label: string; selected: boolean; disabled?: boolean; onPress: () => void }) {
  return (
    <Pressable disabled={disabled} onPress={onPress} style={[styles.kindButton, selected && styles.kindButtonActive, disabled && styles.kindButtonDisabled]}>
      <Text style={[styles.kindButtonText, selected && styles.kindButtonTextActive]}>{label}</Text>
    </Pressable>
  );
}

function statusIcon(status: string) {
  if (status === 'completed') return <CheckCircle2 size={24} color="#22c55e" fill="#ffffff" />;
  if (status === 'assigned') return <Clock size={24} color="#3b82f6" fill="#ffffff" />;
  return <Circle size={24} color="#d4d4d4" fill="#ffffff" />;
}

const styles = StyleSheet.create({
  screen: { flex: 1, backgroundColor: '#ffffff', padding: 24, paddingTop: 64 },
  header: { flexDirection: 'row', alignItems: 'center', gap: 16, marginBottom: 32 },
  backButton: { width: 40, height: 40, borderRadius: 20, backgroundColor: '#f5f5f5', alignItems: 'center', justifyContent: 'center' },
  title: { fontSize: 24, fontWeight: '800', letterSpacing: -0.8, color: '#171717' },
  scroller: { flex: 1 },
  timeline: { gap: 28, paddingBottom: 96 },
  createPanel: { backgroundColor: '#f5f5f5', borderRadius: 28, padding: 18, gap: 12 },
  panelHeaderRow: { flexDirection: 'row', alignItems: 'center', justifyContent: 'space-between', gap: 12 },
  panelTitle: { color: '#171717', fontSize: 20, fontWeight: '900', letterSpacing: -0.3 },
  cancelEditButton: { width: 32, height: 32, borderRadius: 16, backgroundColor: '#ffffff', alignItems: 'center', justifyContent: 'center' },
  input: { minHeight: 52, borderRadius: 16, backgroundColor: '#ffffff', color: '#171717', paddingHorizontal: 14, paddingVertical: 12, fontWeight: '700' },
  notesInput: { minHeight: 96, textAlignVertical: 'top', lineHeight: 20 },
  kindRow: { flexDirection: 'row', gap: 10 },
  kindButton: { flex: 1, borderRadius: 16, borderWidth: 1, borderColor: '#e5e5e5', backgroundColor: '#ffffff', paddingVertical: 13, alignItems: 'center' },
  kindButtonActive: { backgroundColor: '#171717', borderColor: '#171717' },
  kindButtonDisabled: { opacity: 0.55 },
  kindButtonText: { color: '#171717', fontWeight: '900' },
  kindButtonTextActive: { color: '#ffffff' },
  timeInputsRow: { flexDirection: 'row', gap: 10 },
  timeInput: { flex: 1 },
  createButton: { minHeight: 52, borderRadius: 16, backgroundColor: '#171717', alignItems: 'center', justifyContent: 'center', flexDirection: 'row', gap: 8 },
  createButtonDisabled: { opacity: 0.45 },
  createButtonText: { color: '#ffffff', fontWeight: '900' },
  helpText: { color: '#737373', fontSize: 12, lineHeight: 18 },
  message: { color: '#737373', fontWeight: '700' },
  error: { color: '#dc2626', fontWeight: '700', lineHeight: 20 },
  taskRow: { flexDirection: 'row', gap: 20, position: 'relative' },
  time: { width: 48, textAlign: 'right', color: '#a3a3a3', fontSize: 12, fontWeight: '800' },
  iconWrap: { marginTop: 2, backgroundColor: '#ffffff', zIndex: 2 },
  taskBody: { flex: 1, paddingBottom: 8 },
  completed: { opacity: 0.5 },
  taskTitle: { fontSize: 18, fontWeight: '800', color: '#171717', marginBottom: 6 },
  assignee: { alignSelf: 'flex-start', backgroundColor: '#f5f5f5', color: '#525252', fontSize: 12, fontWeight: '600', paddingHorizontal: 10, paddingVertical: 5, borderRadius: 999, overflow: 'hidden' },
  notes: { color: '#737373', lineHeight: 20, marginTop: 8 },
  actionsRow: { flexDirection: 'row', gap: 8, marginTop: 12 },
  actionButton: { backgroundColor: '#171717', borderRadius: 999, paddingHorizontal: 14, paddingVertical: 8 },
  actionButtonText: { color: '#ffffff', fontSize: 12, fontWeight: '800' },
  actionButtonMuted: { backgroundColor: '#f5f5f5', borderRadius: 999, paddingHorizontal: 14, paddingVertical: 8 },
  actionButtonMutedText: { color: '#525252', fontSize: 12, fontWeight: '800' },
});
