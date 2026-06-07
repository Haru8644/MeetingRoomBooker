import { useEffect, useMemo, useState } from 'react'
import './App.css'
import { ApiError } from './lib/apiClient'
import { fetchRoomConflictSummary } from './features/roomConflicts/roomConflictApi'
import type { RoomConflictRecordSummary } from './features/roomConflicts/types'

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

function App() {
    const [summary, setSummary] =
        useState<RoomConflictRecordSummary>(emptySummary)
    const [isLoading, setIsLoading] = useState(true)
    const [errorMessage, setErrorMessage] = useState<string | null>(null)

    useEffect(() => {
        let isMounted = true

        async function loadSummary() {
            try {
                const result = await fetchRoomConflictSummary()

                if (!isMounted) {
                    return
                }

                setSummary(result)
                setErrorMessage(null)
            } catch (error) {
                if (!isMounted) {
                    return
                }

                if (error instanceof ApiError) {
                    setErrorMessage(`Failed to load summary. Status: ${error.status}.`)
                    return
                }

                setErrorMessage('Failed to load summary.')
            } finally {
                if (isMounted) {
                    setIsLoading(false)
                }
            }
        }

        loadSummary()

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
                    {isLoading && <span>Loading summary...</span>}
                    {!isLoading && !errorMessage && <span>Summary loaded from API.</span>}
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

            <section className="operations-panel" aria-labelledby="operations-title">
                <div>
                    <p className="eyebrow">Current phase</p>
                    <h2 id="operations-title">Summary API integration</h2>
                    <p>
                        The summary cards are now connected to the room conflict summary API.
                        List views, filters, manual reporting, and review actions will be
                        added in later pull requests.
                    </p>
                </div>

                <div className="system-notes">
                    <div>
                        <span className="note-label">Worker</span>
                        <span className="note-value">Disabled by default</span>
                    </div>
                    <div>
                        <span className="note-label">API</span>
                        <span className="note-value">Summary connected</span>
                    </div>
                    <div>
                        <span className="note-label">Deployment</span>
                        <span className="note-value">Not connected yet</span>
                    </div>
                </div>
            </section>
        </main>
    )
}

export default App
