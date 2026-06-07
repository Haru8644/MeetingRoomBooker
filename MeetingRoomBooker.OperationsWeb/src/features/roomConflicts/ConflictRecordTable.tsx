import {
    getConflictCauseLabel,
    getConflictImpactLabel,
    getConflictRecordTypeLabel,
    getConflictStatusLabel,
    getConflictStatusTone,
} from './roomConflictLabels'
import { formatDateTime } from './roomConflictFormatters'
import type { RoomConflictRecord } from './types'

type ConflictRecordTableProps = {
    records: RoomConflictRecord[]
    selectedRecordId: number | null
    onSelectRecord: (record: RoomConflictRecord) => void
}

export function ConflictRecordTable({
    records,
    selectedRecordId,
    onSelectRecord,
}: ConflictRecordTableProps) {
    return (
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
                        <th>Action</th>
                    </tr>
                </thead>
                <tbody>
                    {records.map((record) => (
                        <tr
                            className={
                                record.id === selectedRecordId ? 'record-row-selected' : ''
                            }
                            key={record.id}
                        >
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
                            <td>
                                <button
                                    className="table-action"
                                    type="button"
                                    onClick={() => onSelectRecord(record)}
                                >
                                    Select
                                </button>
                            </td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    )
}
