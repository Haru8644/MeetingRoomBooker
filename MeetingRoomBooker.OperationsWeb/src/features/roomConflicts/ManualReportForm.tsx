import type { FormEvent } from 'react'
import {
    getConflictCauseLabel,
    getConflictImpactLabel,
} from './roomConflictLabels'
import {
    conflictCause,
    conflictImpact,
} from './types'
import type {
    ConflictCause,
    ConflictImpact,
} from './types'

export type ManualReportFormValue = {
    occurredAt: string
    roomName: string
    impact: ConflictImpact
    cause: ConflictCause
    description: string
    resolution: string
}

type ManualReportFormProps = {
    value: ManualReportFormValue
    isSaving: boolean
    onValueChange: (value: ManualReportFormValue) => void
    onSubmit: (event: FormEvent<HTMLFormElement>) => void
}

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

export function ManualReportForm({
    value,
    isSaving,
    onValueChange,
    onSubmit,
}: ManualReportFormProps) {
    return (
        <article className="management-card">
            <p className="eyebrow">Manual report</p>
            <h2>Report actual collision</h2>
            <p className="management-description">
                Register a conflict that actually happened in the workplace.
            </p>

            <form className="form-stack" onSubmit={onSubmit}>
                <label className="field">
                    <span>Occurred at</span>
                    <input
                        type="datetime-local"
                        value={value.occurredAt}
                        onChange={(event) =>
                            onValueChange({
                                ...value,
                                occurredAt: event.target.value,
                            })
                        }
                    />
                </label>

                <label className="field">
                    <span>Room name</span>
                    <input
                        type="text"
                        value={value.roomName}
                        placeholder="Large meeting room"
                        onChange={(event) =>
                            onValueChange({
                                ...value,
                                roomName: event.target.value,
                            })
                        }
                    />
                </label>

                <div className="form-grid">
                    <label className="field">
                        <span>Impact</span>
                        <select
                            value={value.impact}
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

                    <label className="field">
                        <span>Cause</span>
                        <select
                            value={value.cause}
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
                </div>

                <label className="field">
                    <span>Description</span>
                    <textarea
                        value={value.description}
                        placeholder="What happened?"
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
                        placeholder="How was it handled?"
                        onChange={(event) =>
                            onValueChange({
                                ...value,
                                resolution: event.target.value,
                            })
                        }
                    />
                </label>

                <button className="primary-action" type="submit" disabled={isSaving}>
                    {isSaving ? 'Saving...' : 'Create report'}
                </button>
            </form>
        </article>
    )
}
