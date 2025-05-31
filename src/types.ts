interface ProxyMessageBase {
  type: string;
  uuid: string;
}

export interface ProxyRequest extends ProxyMessageBase {
  type: "request";

  method: string;
  path: string; // Full path, including query parameters
  body?: string;
}

export interface ProxyResponseHeaders extends ProxyMessageBase {
  type: "response-headers";

  status: number;
  statusText: string;
  headers: Record<string, string>;
}

export interface ProxyResponseChunk extends ProxyMessageBase {
  type: "response-chunk";

  data: string;
  isFinal: boolean;
}

export type ProxyMessageUnion =
  | ProxyRequest
  | ProxyResponseHeaders
  | ProxyResponseChunk;

export type ProxyMessageType = ProxyMessageUnion["type"];
