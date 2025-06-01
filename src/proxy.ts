import {
  ProxyMessageUnion,
  ProxyRequest,
  ProxyResponseHeaders,
} from "./types.ts";

/**
 * Defines the structure for a request that is waiting for a response.
 * We store the 'resolve' and 'reject' functions of the headers promise,
 * and the controller for the response body's ReadableStream.
 */
interface PendingRequest {
  resolveHeaders: (headers: ProxyResponseHeaders) => void;
  reject: (reason?: unknown) => void;
  streamController: ReadableStreamDefaultController<Uint8Array>;
}

export class ProxyManager {
  private static socket: WebSocket | null = null;
  private static textEncoder = new TextEncoder();

  // A simple Map to track requests by their UUID.
  private static pendingRequests = new Map<string, PendingRequest>();

  /**
   * The central message handler. It receives all messages from the client,
   * looks up the corresponding pending request, and routes the data.
   */
  private static handleMessage(event: MessageEvent) {
    try {
      const message: ProxyMessageUnion = JSON.parse(event.data);
      if (!message.uuid) return;

      const pending = this.pendingRequests.get(message.uuid);
      if (!pending) {
        console.warn(
          `Received message for unknown request UUID: ${message.uuid}`,
        );
        return;
      }

      switch (message.type) {
        case "response-headers":
          pending.resolveHeaders(message);
          break;

        case "response-chunk": {
          if (message.data) {
            pending.streamController.enqueue(
              this.textEncoder.encode(message.data),
            );
          }
          if (message.isFinal) {
            pending.streamController.close();
            // The request is complete, clean up the map.
            this.pendingRequests.delete(message.uuid);
          }
          break;
        }
      }
    } catch (error) {
      console.error("Failed to parse or handle proxy message:", error);
    }
  }

  private static handle(req: Request): Response {
    if (req.headers.get("upgrade") !== "websocket") {
      return new Response("Expected websocket upgrade", { status: 426 });
    }

    if (this.isConnected) {
      this.socket?.close(1000, "New connection established");
    }

    const { socket, response } = Deno.upgradeWebSocket(req);
    this.socket = socket;

    socket.onopen = () => console.log("Proxy client connected.");
    socket.onmessage = (event) => this.handleMessage(event);
    socket.onerror = (e) => console.error("Proxy client error:", e);
    socket.onclose = () => {
      console.log("Proxy client disconnected.");
      // When the client disconnects, fail all pending requests.
      for (const [_uuid, pending] of this.pendingRequests.entries()) {
        pending.reject(new Error("Proxy client disconnected."));
        pending.streamController.error(
          new Error("Proxy client disconnected."),
        );
      }
      this.pendingRequests.clear();
      this.socket = null;
    };

    return response;
  }

  static handler = this.handle.bind(this);

  static get isConnected(): boolean {
    return this.socket !== null && this.socket.readyState === WebSocket.OPEN;
  }

  static async request(
    method: string,
    path: string,
    body?: string,
  ): Promise<Response> {
    if (!this.isConnected) {
      return new Response("Proxy client not connected", { status: 503 });
    }

    const uuid = crypto.randomUUID();
    let responseStream: ReadableStream<Uint8Array>;

    const headersPromise = new Promise<ProxyResponseHeaders>(
      (resolve, reject) => {
        const timeout = setTimeout(() => {
          // Clean up and reject if the client doesn't send headers in time.
          this.pendingRequests.delete(uuid);
          reject(new Error("Proxy request timed out waiting for headers."));
        }, 900e3); // 15 minutes timeout

        responseStream = new ReadableStream({
          start: (controller) => {
            // Store the callbacks and controller in our map.
            this.pendingRequests.set(uuid, {
              resolveHeaders: (headers) => {
                clearTimeout(timeout);
                resolve(headers);
              },
              reject: (reason) => {
                clearTimeout(timeout);
                reject(reason);
              },
              streamController: controller,
            });
          },
          cancel: () => {
            // If the consumer of the response cancels reading, clean up.
            console.log(`Request ${uuid} stream cancelled.`);
            this.pendingRequests.delete(uuid);
          },
        });
      },
    );

    // Send the request to the client.
    const requestMessage: ProxyRequest = {
      type: "request",
      uuid,
      method,
      path,
      body,
    };
    this.socket!.send(JSON.stringify(requestMessage));

    try {
      // Wait for the headers to arrive.
      const { status, statusText, headers } = await headersPromise;
      // Return a new response with the streaming body.
      return new Response(responseStream!, { status, statusText, headers });
    } catch (error) {
      console.error(`Proxy request ${uuid} failed:`, error);
      return new Response(
        error instanceof Error ? error.message : String(error),
        {
          status: 504,
        },
      ); // 504 Gateway Timeout
    }
  }
}
