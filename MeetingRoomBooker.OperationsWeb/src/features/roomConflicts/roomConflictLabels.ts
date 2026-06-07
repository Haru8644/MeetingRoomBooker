import {
    conflictCause,
    conflictImpact,
    conflictRecordType,
    conflictStatus,
} from './types'
import type {
    ConflictCause,
    ConflictImpact,
    ConflictRecordType,
    ConflictStatus,
} from './types'

const recordTypeLabels: Record<ConflictRecordType, string> = {
    [conflictRecordType.unresolvedReservationOverlap]: 'Detected overlap',
    [conflictRecordType.actualRoomCollision]: 'Actual collision',
}

const statusLabels: Record<ConflictStatus, string> = {
    [conflictStatus.detected]: 'Detected',
    [conflictStatus.confirmed]: 'Confirmed',
    [conflictStatus.falseAlarm]: 'False alarm',
    [conflictStatus.resolved]: 'Resolved',
}

const impactLabels: Record<ConflictImpact, string> = {
    [conflictImpact.low]: 'Low',
    [conflictImpact.medium]: 'Medium',
    [conflictImpact.high]: 'High',
}

const causeLabels: Record<ConflictCause, string> = {
    [conflictCause.existingReservationOverlooked]: 'Existing reservation overlooked',
    [conflictCause.externalCalendarConflict]: 'External calendar conflict',
    [conflictCause.inputMistake]: 'Input mistake',
    [conflictCause.notificationMissed]: 'Notification missed',
    [conflictCause.lastMinuteChange]: 'Last-minute change',
    [conflictCause.verbalReservation]: 'Verbal reservation',
    [conflictCause.unknown]: 'Unknown',
    [conflictCause.other]: 'Other',
}

export function getConflictRecordTypeLabel(type: ConflictRecordType): string {
    return recordTypeLabels[type]
}

export function getConflictStatusLabel(status: ConflictStatus): string {
    return statusLabels[status]
}

export function getConflictImpactLabel(impact: ConflictImpact): string {
    return impactLabels[impact]
}

export function getConflictCauseLabel(cause: ConflictCause): string {
    return causeLabels[cause]
}

export function getConflictStatusTone(status: ConflictStatus): string {
    if (status === conflictStatus.detected) {
        return 'detected'
    }

    if (status === conflictStatus.confirmed) {
        return 'confirmed'
    }

    if (status === conflictStatus.falseAlarm) {
        return 'false-alarm'
    }

    return 'resolved'
}
