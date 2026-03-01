export interface RpcRequest {
  id: string | number;
  method: string;
  params?: Record<string, unknown>;
}

export interface RpcResponse {
  id: string | number | null;
  result?: unknown;
  error?: { code: number; message: string };
}

export interface ServerEvent {
  event: string;
  data: unknown;
  ts: string;
}

export type ServerMessage = RpcResponse | ServerEvent;
