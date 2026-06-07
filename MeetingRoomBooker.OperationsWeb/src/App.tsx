import { useEffect, useMemo, useState } from 'react'
import './App.css'
import { ApiError } from './lib/apiClient'
import {
    fetchRoomConflictRecords,
    fetchRoomConflictSummary,
} from './features/roomConflicts/roomConflictApi'
import {
    getConflictCauseLabel,
    getConflictImpactLabel,
    getConflictRecordTypeLabel,
    getConflictStatusLabel,
    getConflictStatusTone,
} from './features/roomConflicts/roomConflictLabels'
import type {
    RoomConflictRecord,
    RoomConflictRecordSummary,
} from './features/roomConflicts/types'

type SummaryCard = {
    label: string
    value: number
    description: string
}

const emptySummary: RoomConflictRecordSummary = {
    unresolvedOverlapsThisMonth: 0,
    confirmedCollisionsThisMonth: 0,
    highImpactConflictsThisMonth: 0,
    openDetectedRecords: 0,
}

const dateTimeFormatter = new Intl.DateTimeFormat('ja-JP', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
})

function buildSummaryCards(summary: RoomConflictRecordSummary): SummaryCard[] {
    return [
        {
            label: 'Detected overlaps',
            value: summary.unresolvedOverlapsThisMonth,
            description: 'Unresolved reservation overlaps detected by the worker.',
        },
        {
            label: 'Confirmed collisions',
            value: summary.confirmedCollisionsThisMonth,
            description: 'Room conflicts confirmed as actual workplace collisions.',
        },
        {
            label: 'High impact',
            value: summary.highImpactConflictsThisMonth,
            description: 'Conflicts that affected meetings or required urgent action.',
        },
        {
            label: 'Open records',
            value: summary.openDetectedRecords,
            description: 'Detected records that still need review or classification.',
        },
    ]
}

function formatDateTime(value: string): string {
    const date = new Date(value)

    if (Number.isNaN(date.getTime())) {
        return value
    }

    return dateTimeFormatter.format(date)
}

function App() {
    const [summary, setSummary] =
        useState<RoomConflictRecordSummary>(emptySummary)
    const [records, setRecords] = useState<RoomConflictRecord[]>([])
    const [isLoading, setIsLoading] = useState(true)
    const [errorMessage, setErrorMessage] = useState<string | null>(null)

    useEffect(() => {
        let isMounted = true

        async function loadDashboardData() {
            try {
                const [summaryResult, recordsResult] = await Promise.all([
                    fetchRoomConflictSummary(),
                    fetchRoomConflictRecords(),
                ])

                if (!isMounted) {
                    return
                }

                setSummary(summaryResult)
                setRecords(recordsResult)
                setErrorMessage(null)
            } catch (error) {
                if (!isMounted) {
                    return
                }

                if (error instanceof ApiError) {
                    setErrorMessage(`Failed to load dashboard data. Status: ${error.status}.`)
                    return
                }

                setErrorMessage('Failed to load dashboard data.')
            } finally {
                if (isMounted) {
                    setIsLoading(false)
                }
            }
        }

        loadDashboardData()

        return () => {
            isMounted = false
        }
    }, [])

    const summaryCards = useMemo(() => buildSummaryCards(summary), [summary])

    return (
        <main className="app-shell">
            <section className="hero-section" aria-labelledby="page-title">
                <p className="eyebrow">MeetingRoomBooker Operations</p>
                <h1 id="page-title">Room Conflict Tracking</h1>
                <p className="hero-description">
                    Track unresolved reservation overlaps, review actual room collisions,
                    and turn operational friction into measurable improvement.
                </p>

                <div className="feedback-row" aria-live="polite">
                    {isLoading && <span>Loading dashboard data...</span>}
                    {!isLoading && !errorMessage && <span>Dashboard data loaded from API.</span>}
                    {errorMessage && <span className="error-message">{errorMessage}</span>}
                </div>
            </section>

            <section className="status-grid" aria-label="Conflict tracking summary">
                {summaryCards.map((card) => (
                    <article className="summary-card" key={card.label}>
                        <p className="summary-label">{card.label}</p>
                        <strong className="summary-value">{card.value}</strong>
                        <p className="summary-description">{card.description}</p>
                    </article>
                ))}
            </section>

            <section className="records-panel" aria-labelledby="records-title">
                <div className="records-header">
                    <div>
                        <p className="eyebrow">Conflict records</p>
                        <h2 id="records-title">Latest room conflict records</h2>
                    </div>
                    <span className="record-count">{records.length} records</span>
                </div>

                {isLoading && <p className="empty-state">Loading conflict records...</p>}

                {!isLoading && errorMessage && (
                    <p className="empty-state">
                        Conflict records could not be loaded. Check the API server and login state.
                    </p>
                )}

                {!isLoading && !errorMessage && records.length === 0 && (
                    <p className="empty-state">No conflict records found.</p>
                )}

                {!isLoading && !errorMessage && records.length > 0 && (
                    <div className="records-table-wrap">
                        <table className="records-table">
                            <thead>
                                <tr>
                                    <th>Status</th>
                                    <th>Occurred at</th>
                                    <th>Room</th>
                                    <th>Type</th>
                                    <th>Impact</th>
                                    <th>Cause</th>
                                    <th>Permission</th>
                                </tr>
                            </thead>
                            <tbody>
                                {records.map((record) => (
                                    <tr key={record.id}>
                                        <td>
                                            <span
                                                className={`status-pill status-${getConflictStatusTone(
                                                    record.status,
                                                )}`}
                                            >
                                                {getConflictStatusLabel(record.status)}
                                            </span>
                                        </td>
                                        <td>{formatDateTime(record.occurredAt)}</td>
                                        <td>{record.roomName}</td>
                                        <td>{getConflictRecordTypeLabel(record.type)}</td>
                                        <td>{getConflictImpactLabel(record.impact)}</td>
                                        <td>{getConflictCauseLabel(record.cause)}</td>
                                        <td>{record.canEdit ? 'Editable' : 'View only'}</td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </div>
                )}
            </section>

            <section className="operations-panel" aria-labelledby="operations-title">
                <div>
                    <p className="eyebrow">Current phase</p>
                    <h2 id="operations-title">Conflict record list</h2>
                    <p>
                        The operations UI now loads summary metrics and read-only conflict
                        records from the API. Manual reporting, filtering, and review actions
                        will be added in later pull requests.
                    </p>
                </div>

                <div className="system-notes">
                    <div>
                        <span className="note-label">Worker</span>
                        <span className="note-value">Disabled by default</span>
                    </div>
                    <div>
                        <span className="note-label">API</span>
                        <span className="note-value">Summary and list connected</span>
                    </div>
                    <div>
                        <span className="note-label">Mode</span>
                        <span className="note-value">Read only</span>
                    </div>
                </div>
            </section>
        </main>
    )
}

export default App
