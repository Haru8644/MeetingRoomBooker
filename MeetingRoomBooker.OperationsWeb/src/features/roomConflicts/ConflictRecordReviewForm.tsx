import type { FormEvent } from 'react'
import {
    getConflictCauseLabel,
    getConflictImpactLabel,
    getConflictStatusLabel,
} from './roomConflictLabels'
import { formatDateTime } from './roomConflictFormatters'
import {
    conflictCause,
    conflictImpact,
    conflictStatus,
} from './types'
import type {
    ConflictCause,
    ConflictImpact,
    ConflictStatus,
    RoomConflictRecord,
} from './types'

export type ConflictRecordReviewFormValue = {
    status: ConflictStatus
    impact: ConflictImpact
    cause: ConflictCause
    description: string
    resolution: string
}

type ConflictRecordReviewFormProps = {
    selectedRecord: RoomConflictRecord | null
    value: ConflictRecordReviewFormValue | null
    isSaving: boolean
    onValueChange: (value: ConflictRecordReviewFormValue) => void
    onSubmit: (event: FormEvent<HTMLFormElement>) => void
}

const statusOptions: ConflictStatus[] = [
    conflictStatus.detected,
    conflictStatus.confirmed,
    conflictStatus.falseAlarm,
    conflictStatus.resolved,
]

const impactOptions: ConflictImpact[] = [
    conflictImpact.low,
    conflictImpact.medium,
    conflictImpact.high,
]

const causeOptions: ConflictCause[] = [
    conflictCause.existingReservationOverlooked,
    conflictCause.externalCalendarConflict,
    conflictCause.inputMistake,
    conflictCause.notificationMissed,
    conflictCause.lastMinuteChange,
    conflictCause.verbalReservation,
    conflictCause.unknown,
    conflictCause.other,
]

export function ConflictRecordReviewForm({
    selectedRecord,
    value,
    isSaving,
    onValueChange,
    onSubmit,
}: ConflictRecordReviewFormProps) {
    if (!selectedRecord || !value) {
        return (
            <article className="management-card">
                <p className="eyebrow">Review</p>
                <h2>Classify selected record</h2>
                <p className="management-description">
                    Select an editable record from the list, then update its review status.
                </p>

                <p className="empty-state">No conflict record selected.</p>
            </article>
        )
    }

    return (
        <article className="management-card">
            <p className="eyebrow">Review</p>
            <h2>Classify selected record</h2>
            <p className="management-description">
                Select an editable record from the list, then update its review status.
            </p>

            <form className="form-stack" onSubmit={onSubmit}>
                <div className="selected-record-summary">
                    <span>{selectedRecord.roomName}</span>
                    <strong>{formatDateTime(selectedRecord.occurredAt)}</strong>
                </div>

                <div className="form-grid">
                    <label className="field">
                        <span>Status</span>
                        <select
                            value={value.status}
                            disabled={!selectedRecord.canEdit}
                            onChange={(event) =>
                                onValueChange({
                                    ...value,
                                    status: Number(event.target.value) as ConflictStatus,
                                })
                            }
                        >
                            {statusOptions.map((status) => (
                                <option key={status} value={status}>
                                    {getConflictStatusLabel(status)}
                                </option>
                            ))}
                        </select>
                    </label>

                    <label className="field">
                        <span>Impact</span>
                        <select
                            value={value.impact}
                            disabled={!selectedRecord.canEdit}
                            onChange={(event) =>
                                onValueChange({
                                    ...value,
                                    impact: Number(event.target.value) as ConflictImpact,
                                })
                            }
                        >
                            {impactOptions.map((impact) => (
                                <option key={impact} value={impact}>
                                    {getConflictImpactLabel(impact)}
                                </option>
                            ))}
                        </select>
                    </label>
                </div>

                <label className="field">
                    <span>Cause</span>
                    <select
                        value={value.cause}
                        disabled={!selectedRecord.canEdit}
                        onChange={(event) =>
                            onValueChange({
                                ...value,
                                cause: Number(event.target.value) as ConflictCause,
                            })
                        }
                    >
                        {causeOptions.map((cause) => (
                            <option key={cause} value={cause}>
                                {getConflictCauseLabel(cause)}
                            </option>
                        ))}
                    </select>
                </label>

                <label className="field">
                    <span>Description</span>
                    <textarea
                        value={value.description}
                        disabled={!selectedRecord.canEdit}
                        onChange={(event) =>
                            onValueChange({
                                ...value,
                                description: event.target.value,
                            })
                        }
                    />
                </label>

                <label className="field">
                    <span>Resolution</span>
                    <textarea
                        value={value.resolution}
                        disabled={!selectedRecord.canEdit}
                        onChange={(event) =>
                            onValueChange({
                                ...value,
                                resolution: event.target.value,
                            })
                        }
                    />
                </label>

                <button
                    className="primary-action"
                    type="submit"
                    disabled={isSaving || !selectedRecord.canEdit}
                >
                    {isSaving ? 'Saving...' : 'Update record'}
                </button>
            </form>
        </article>
    )
}
