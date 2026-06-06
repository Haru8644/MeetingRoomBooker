export const conflictRecordType = {
    unresolvedReservationOverlap: 0,
    actualRoomCollision: 1,
} as const

export type ConflictRecordType =
    (typeof conflictRecordType)[keyof typeof conflictRecordType]

export const conflictStatus = {
    detected: 0,
    confirmed: 1,
    falseAlarm: 2,
    resolved: 3,
} as const

export type ConflictStatus =
    (typeof conflictStatus)[keyof typeof conflictStatus]

export const conflictImpact = {
    low: 0,
    medium: 1,
    high: 2,
} as const

export type ConflictImpact =
    (typeof conflictImpact)[keyof typeof conflictImpact]

export const conflictCause = {
    existingReservationOverlooked: 0,
    externalCalendarConflict: 1,
    inputMistake: 2,
    notificationMissed: 3,
    lastMinuteChange: 4,
    verbalReservation: 5,
    unknown: 98,
    other: 99,
} as const

export type ConflictCause =
    (typeof conflictCause)[keyof typeof conflictCause]

export type RoomConflictRecord = {
    id: number
    type: ConflictRecordType
    status: ConflictStatus
    occurredAt: string
    roomName: string
    reservationIdA: number | null
    reservationIdB: number | null
    impact: ConflictImpact
    cause: ConflictCause
    description: string
    resolution: string
    detectionKey: string | null
    reportedByUserId: number | null
    createdAt: string
    updatedAt: string | null
    canEdit: boolean
}

export type RoomConflictRecordSummary = {
    unresolvedOverlapsThisMonth: number
    confirmedCollisionsThisMonth: number
    highImpactConflictsThisMonth: number
    openDetectedRecords: number
}

export type CreateRoomConflictRecordRequest = {
    occurredAt: string
    roomName: string
    reservationIdA?: number | null
    reservationIdB?: number | null
    impact: ConflictImpact
    cause: ConflictCause
    description?: string | null
    resolution?: string | null
    status?: ConflictStatus | null
}

export type UpdateRoomConflictRecordRequest = {
    status: ConflictStatus
    impact: ConflictImpact
    cause: ConflictCause
    description?: string | null
    resolution?: string | null
}
