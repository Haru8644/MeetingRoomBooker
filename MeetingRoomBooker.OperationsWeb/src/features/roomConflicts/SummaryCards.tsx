import type { RoomConflictRecordSummary } from './types'

type SummaryCard = {
    label: string
    value: number
    description: string
}

type SummaryCardsProps = {
    summary: RoomConflictRecordSummary
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

export function SummaryCards({ summary }: SummaryCardsProps) {
    const summaryCards = buildSummaryCards(summary)

    return (
        <section className="status-grid" aria-label="Conflict tracking summary">
            {summaryCards.map((card) => (
                <article className="summary-card" key={card.label}>
                    <p className="summary-label">{card.label}</p>
                    <strong className="summary-value">{card.value}</strong>
                    <p className="summary-description">{card.description}</p>
                </article>
            ))}
        </section>
    )
}
