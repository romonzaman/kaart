import type {
  Card,
  CardList,
  CreateCardInput,
  CreateDeckInput,
  Deck,
  DeckList,
  DeckStats,
  Health,
  ListCardsParams,
  Queue,
  ReviewInput,
  ReviewResult,
  UpdateCardInput,
  UpdateDeckInput,
} from './types';

/**
 * The base URL of the local kaartd process. Overridable so a device on the same
 * network can point at a laptop running the server.
 */
export const API_URL = process.env.EXPO_PUBLIC_API_URL ?? 'http://localhost:8080';

/** The error codes kaartd returns. Anything else is normalised to 'unknown'. */
export type ApiErrorCode =
  | 'invalid_request'
  | 'not_found'
  | 'conflict'
  | 'internal'
  | 'network'
  | 'unknown';

/**
 * Every failure reaching a component is an ApiError — a failed fetch, a non-2xx
 * response, and an unparseable body all arrive in the same shape, so screens
 * never branch on how something went wrong.
 */
export class ApiError extends Error {
  readonly code: ApiErrorCode;
  readonly status: number;

  constructor(code: ApiErrorCode, message: string, status = 0) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
  }

  /** True when retrying later could plausibly succeed. */
  get retryable(): boolean {
    return this.code === 'network' || this.code === 'internal';
  }
}

type ErrorBody = {
  error?: {
    code?: string;
    message?: string;
  };
};

const KNOWN_CODES: ReadonlySet<string> = new Set([
  'invalid_request',
  'not_found',
  'conflict',
  'internal',
]);

function normaliseCode(raw: string | undefined): ApiErrorCode {
  if (raw !== undefined && KNOWN_CODES.has(raw)) {
    return raw as ApiErrorCode;
  }
  return 'unknown';
}

async function request<T>(
  path: string,
  init?: { method?: string; body?: unknown; signal?: AbortSignal },
): Promise<T> {
  const method = init?.method ?? 'GET';

  let response: Response;
  try {
    response = await fetch(`${API_URL}${path}`, {
      method,
      headers: init?.body === undefined ? undefined : { 'Content-Type': 'application/json' },
      body: init?.body === undefined ? undefined : JSON.stringify(init.body),
      signal: init?.signal,
    });
  } catch (cause) {
    const detail = cause instanceof Error ? cause.message : String(cause);
    throw new ApiError(
      'network',
      `Could not reach Kaart at ${API_URL}. Is the server running? (${detail})`,
    );
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const text = await response.text();

  if (!response.ok) {
    let body: ErrorBody = {};
    try {
      body = JSON.parse(text) as ErrorBody;
    } catch {
      // A non-JSON error body means something other than kaartd answered.
    }
    throw new ApiError(
      normaliseCode(body.error?.code),
      body.error?.message ?? `Request failed with status ${response.status}`,
      response.status,
    );
  }

  if (text === '') {
    return undefined as T;
  }

  try {
    return JSON.parse(text) as T;
  } catch {
    throw new ApiError('unknown', 'The server returned a response that could not be read.', response.status);
  }
}

function query(params: Record<string, string | number | undefined>): string {
  const parts: string[] = [];
  for (const [key, value] of Object.entries(params)) {
    if (value === undefined || value === '') continue;
    parts.push(`${encodeURIComponent(key)}=${encodeURIComponent(String(value))}`);
  }
  return parts.length === 0 ? '' : `?${parts.join('&')}`;
}

/** One function per endpoint. No component builds a URL itself. */
export const api = {
  health: (signal?: AbortSignal) => request<Health>('/healthz', { signal }),

  listDecks: (signal?: AbortSignal) => request<DeckList>('/api/v1/decks', { signal }),

  getDeck: (deckId: string, signal?: AbortSignal) =>
    request<Deck>(`/api/v1/decks/${encodeURIComponent(deckId)}`, { signal }),

  createDeck: (input: CreateDeckInput) =>
    request<Deck>('/api/v1/decks', { method: 'POST', body: input }),

  updateDeck: (deckId: string, input: UpdateDeckInput) =>
    request<Deck>(`/api/v1/decks/${encodeURIComponent(deckId)}`, { method: 'PATCH', body: input }),

  deleteDeck: (deckId: string) =>
    request<void>(`/api/v1/decks/${encodeURIComponent(deckId)}`, { method: 'DELETE' }),

  listCards: (deckId: string, params: ListCardsParams = {}, signal?: AbortSignal) =>
    request<CardList>(
      `/api/v1/decks/${encodeURIComponent(deckId)}/cards${query({
        q: params.q,
        limit: params.limit,
        offset: params.offset,
      })}`,
      { signal },
    ),

  createCard: (deckId: string, input: CreateCardInput) =>
    request<Card>(`/api/v1/decks/${encodeURIComponent(deckId)}/cards`, {
      method: 'POST',
      body: input,
    }),

  getCard: (cardId: string, signal?: AbortSignal) =>
    request<Card>(`/api/v1/cards/${encodeURIComponent(cardId)}`, { signal }),

  updateCard: (cardId: string, input: UpdateCardInput) =>
    request<Card>(`/api/v1/cards/${encodeURIComponent(cardId)}`, { method: 'PATCH', body: input }),

  deleteCard: (cardId: string) =>
    request<void>(`/api/v1/cards/${encodeURIComponent(cardId)}`, { method: 'DELETE' }),

  suspendCard: (cardId: string) =>
    request<Card>(`/api/v1/cards/${encodeURIComponent(cardId)}/suspend`, { method: 'POST' }),

  unsuspendCard: (cardId: string) =>
    request<Card>(`/api/v1/cards/${encodeURIComponent(cardId)}/unsuspend`, { method: 'POST' }),

  getQueue: (deckId: string, limit = 50, signal?: AbortSignal) =>
    request<Queue>(`/api/v1/decks/${encodeURIComponent(deckId)}/queue${query({ limit })}`, {
      signal,
    }),

  getStats: (deckId: string, days?: number, signal?: AbortSignal) =>
    request<DeckStats>(`/api/v1/decks/${encodeURIComponent(deckId)}/stats${query({ days })}`, {
      signal,
    }),

  review: (cardId: string, input: ReviewInput) =>
    request<ReviewResult>(`/api/v1/cards/${encodeURIComponent(cardId)}/review`, {
      method: 'POST',
      body: input,
    }),
};

export type Api = typeof api;
