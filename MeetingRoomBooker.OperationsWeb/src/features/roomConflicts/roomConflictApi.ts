import { getJson, sendJson } from '../../lib/apiClient'
import type {
    CreateRoomConflictRecordRequest,
    RoomConflictRecord,
    RoomConflictRecordSummary,
    UpdateRoomConflictRecordRequest,
} from './types'

const basePath = '/api/room-conflict-records'

export function fetchRoomConflictRecords(): Promise<RoomConflictRecord[]> {
    return getJson<RoomConflictRecord[]>(basePath)
}

export function fetchRoomConflictRecord(
    id: number,
): Promise<RoomConflictRecord> {
    return getJson<RoomConflictRecord>(`${basePath}/${id}`)
}

export function fetchRoomConflictSummary(): Promise<RoomConflictRecordSummary> {
    return getJson<RoomConflictRecordSummary>(`${basePath}/summary`)
}

export function createRoomConflictRecord(
    request: CreateRoomConflictRecordRequest,
): Promise<RoomConflictRecord> {
    return sendJson<CreateRoomConflictRecordRequest, RoomConflictRecord>(
        basePath,
        'POST',
        request,
    )
}

export function updateRoomConflictRecord(
    id: number,
    request: UpdateRoomConflictRecordRequest,
): Promise<RoomConflictRecord> {
    return sendJson<UpdateRoomConflictRecordRequest, RoomConflictRecord>(
        `${basePath}/${id}`,
        'PUT',
        request,
    )
}
