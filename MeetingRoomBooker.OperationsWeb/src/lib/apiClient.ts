export class ApiError extends Error {
    public readonly status: number

    constructor(message: string, status: number) {
        super(message)
        this.name = 'ApiError'
        this.status = status
    }
}

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? ''

export async function getJson<TResponse>(path: string): Promise<TResponse> {
    const response = await fetch(`${apiBaseUrl}${path}`, {
        method: 'GET',
        credentials: 'include',
        headers: {
            Accept: 'application/json',
        },
    })

    if (!response.ok) {
        throw new ApiError(
            `API request failed with status ${response.status}.`,
            response.status,
        )
    }

    return (await response.json()) as TResponse
}

export async function sendJson<TRequest, TResponse>(
    path: string,
    method: 'POST' | 'PUT',
    body: TRequest,
): Promise<TResponse> {
    const response = await fetch(`${apiBaseUrl}${path}`, {
        method,
        credentials: 'include',
        headers: {
            Accept: 'application/json',
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(body),
    })

    if (!response.ok) {
        throw new ApiError(
            `API request failed with status ${response.status}.`,
            response.status,
        )
    }

    return (await response.json()) as TResponse
}
