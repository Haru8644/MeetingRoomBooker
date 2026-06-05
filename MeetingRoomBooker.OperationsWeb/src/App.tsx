import './App.css'

type SummaryCard = {
    label: string
    value: string
    description: string
}

const summaryCards: SummaryCard[] = [
    {
        label: 'Detected overlaps',
        value: '0',
        description: 'Unresolved reservation overlaps detected by the worker.',
    },
    {
        label: 'Confirmed collisions',
        value: '0',
        description: 'Room conflicts confirmed as actual workplace collisions.',
    },
    {
        label: 'High impact',
        value: '0',
        description: 'Conflicts that affected meetings or required urgent action.',
    },
    {
        label: 'Open records',
        value: '0',
        description: 'Detected records that still need review or classification.',
    },
]

function App() {
    return (
        <main className="app-shell">
            <section className="hero-section" aria-labelledby="page-title">
                <p className="eyebrow">MeetingRoomBooker Operations</p>
                <h1 id="page-title">Room Conflict Tracking</h1>
                <p className="hero-description">
                    Track unresolved reservation overlaps, review actual room collisions,
                    and turn operational friction into measurable improvement.
                </p>
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
                    <h2 id="operations-title">Operations UI scaffold</h2>
                    <p>
                        This screen is the starting point for the future conflict tracking
                        UI. API integration, filtering, manual reporting, and review actions
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
                        <span className="note-value">Ready for integration</span>
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
