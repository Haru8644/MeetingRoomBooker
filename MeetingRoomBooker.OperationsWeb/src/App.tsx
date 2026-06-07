import { useEffect, useMemo, useState } from 'react'
import type { FormEvent } from 'react'
import './App.css'
import { ApiError } from './lib/apiClient'
import {
    createRoomConflictRecord,
    fetchRoomConflictRecords,
    fetchRoomConflictSummary,
    updateRoomConflictRecord,
} from './features/roomConflicts/roomConflictApi'
import { ConflictRecordReviewForm } from './features/roomConflicts/ConflictRecordReviewForm'
import type { ConflictRecordReviewFormValue } from './features/roomConflicts/ConflictRecordReviewForm'
import { ConflictRecordTable } from './features/roomConflicts/ConflictRecordTable'
import { ManualReportForm } from './features/roomConflicts/ManualReportForm'
import type { ManualReportFormValue } from './features/roomConflicts/ManualReportForm'
import { SummaryCards } from './features/roomConflicts/SummaryCards'
import {
    conflictCause,
    conflictImpact,
    conflictStatus,
} from './features/roomConflicts/types'
import type {
    RoomConflictRecord,
    RoomConflictRecordSummary,
} from './features/roomConflicts/types'

const emptySummary: RoomConflictRecordSummary = {
    unresolvedOverlapsThisMonth: 0,
    confirmedCollisionsThisMonth: 0,
    highImpactConflictsThisMonth: 0,
    openDetectedRecords: 0,
}

function formatDateTimeInputValue(date: Date): string {
    const year = date.getFullYear()
    const month = String(date.getMonth() + 1).padStart(2, '0')
    const day = String(date.getDate()).padStart(2, '0')
    const hour = String(date.getHours()).padStart(2, '0')
    const minute = String(date.getMinutes()).padStart(2, '0')

    return `${year}-${month}-${day}T${hour}:${minute}`
}

function createDefaultManualReportForm(): ManualReportFormValue {
    return {
        occurredAt: formatDateTimeInputValue(new Date()),
        roomName: '',
        impact: conflictImpact.medium,
        cause: conflictCause.unknown,
        description: '',
        resolution: '',
    }
}

function createEditForm(record: RoomConflictRecord): ConflictRecordReviewFormValue {
    return {
        status: record.status,
        impact: record.impact,
        cause: record.cause,
        description: record.description ?? '',
        resolution: record.resolution ?? '',
    }
}

function toNullableText(value: string): string | null {
    const trimmedValue = value.trim()

    if (trimmedValue.length === 0) {
        return null
    }

    return trimmedValue
}

async function fetchDashboardData(): Promise<{
    summaryResult: RoomConflictRecordSummary
    recordsResult: RoomConflictRecord[]
}> {
    const [summaryResult, recordsResult] = await Promise.all([
        fetchRoomConflictSummary(),
        fetchRoomConflictRecords(),
    ])

    return {
        summaryResult,
        recordsResult,
    }
}

function App() {
    const [summary, setSummary] =
        useState<RoomConflictRecordSummary>(emptySummary)
    const [records, setRecords] = useState<RoomConflictRecord[]>([])
    const [isLoading, setIsLoading] = useState(true)
    const [errorMessage, setErrorMessage] = useState<string | null>(null)
    const [createForm, setCreateForm] = useState<ManualReportFormValue>(
        createDefaultManualReportForm,
    )
    const [selectedRecordId, setSelectedRecordId] = useState<number | null>(null)
    const [editForm, setEditForm] =
        useState<ConflictRecordReviewFormValue | null>(null)
    const [isSaving, setIsSaving] = useState(false)
    const [saveMessage, setSaveMessage] = useState<string | null>(null)

    useEffect(() => {
        let isMounted = true

        async function loadDashboardData() {
            try {
                const { summaryResult, recordsResult } = await fetchDashboardData()

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

    const selectedRecord = useMemo(
        () => records.find((record) => record.id === selectedRecordId) ?? null,
        [records, selectedRecordId],
    )

    async function reloadDashboardData() {
        const { summaryResult, recordsResult } = await fetchDashboardData()

        setSummary(summaryResult)
        setRecords(recordsResult)
    }

    function handleSelectRecord(record: RoomConflictRecord) {
        setSelectedRecordId(record.id)
        setEditForm(createEditForm(record))
        setSaveMessage(null)
    }

    async function handleCreateSubmit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault()

        if (createForm.roomName.trim().length === 0) {
            setSaveMessage('Room name is required.')
            return
        }

        setIsSaving(true)
        setSaveMessage(null)

        try {
            await createRoomConflictRecord({
                occurredAt: createForm.occurredAt,
                roomName: createForm.roomName.trim(),
                impact: createForm.impact,
                cause: createForm.cause,
                description: toNullableText(createForm.description),
                resolution: toNullableText(createForm.resolution),
                status: conflictStatus.confirmed,
            })

            await reloadDashboardData()
            setCreateForm(createDefaultManualReportForm())
            setSaveMessage('Manual conflict report was created.')
        } catch (error) {
            if (error instanceof ApiError) {
                setSaveMessage(`Failed to create report. Status: ${error.status}.`)
                return
            }

            setSaveMessage('Failed to create report.')
        } finally {
            setIsSaving(false)
        }
    }

    async function handleUpdateSubmit(event: FormEvent<HTMLFormElement>) {
        event.preventDefault()

        if (!selectedRecord || !editForm) {
            setSaveMessage('Select a conflict record first.')
            return
        }

        setIsSaving(true)
        setSaveMessage(null)

        try {
            await updateRoomConflictRecord(selectedRecord.id, {
                status: editForm.status,
                impact: editForm.impact,
                cause: editForm.cause,
                description: toNullableText(editForm.description),
                resolution: toNullableText(editForm.resolution),
            })

            await reloadDashboardData()
            setSaveMessage('Conflict record was updated.')
        } catch (error) {
            if (error instanceof ApiError) {
                setSaveMessage(`Failed to update record. Status: ${error.status}.`)
                return
            }

            setSaveMessage('Failed to update record.')
        } finally {
            setIsSaving(false)
        }
    }

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

            <SummaryCards summary={summary} />

            <section className="management-grid" aria-label="Conflict record management">
                <ManualReportForm
                    value={createForm}
                    isSaving={isSaving}
                    onValueChange={setCreateForm}
                    onSubmit={handleCreateSubmit}
                />

                <ConflictRecordReviewForm
                    selectedRecord={selectedRecord}
                    value={editForm}
                    isSaving={isSaving}
                    onValueChange={setEditForm}
                    onSubmit={handleUpdateSubmit}
                />
            </section>

            {saveMessage && (
                <div className="save-message" aria-live="polite">
                    {saveMessage}
                </div>
            )}

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
                    <ConflictRecordTable
                        records={records}
                        selectedRecordId={selectedRecordId}
                        onSelectRecord={handleSelectRecord}
                    />
                )}
            </section>

            <section className="operations-panel" aria-labelledby="operations-title">
                <div>
                    <p className="eyebrow">Current phase</p>
                    <h2 id="operations-title">Conflict record management</h2>
                    <p>
                        The operations UI can now create manual conflict reports and classify
                        existing conflict records. Filtering, routing, and advanced review
                        workflows can be added after the core management flow is stable.
                    </p>
                </div>

                <div className="system-notes">
                    <div>
                        <span className="note-label">Worker</span>
                        <span className="note-value">Disabled by default</span>
                    </div>
                    <div>
                        <span className="note-label">API</span>
                        <span className="note-value">Create and update connected</span>
                    </div>
                    <div>
                        <span className="note-label">Mode</span>
                        <span className="note-value">Manage records</span>
                    </div>
                </div>
            </section>
        </main>
    )
}

export default App
