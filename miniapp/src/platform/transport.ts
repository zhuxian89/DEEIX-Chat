import Taro from "@tarojs/taro";
import { MemoryRefreshCookieBridge } from "./memory-cookie-bridge";

export type ApiEnvelope<T> = {
  data?: T;
  details?: unknown;
  errorCode?: string;
  errorMsg?: string;
  requestId?: string;
};

export type ApiRequest = {
  path: string;
  method?: "DELETE" | "GET" | "PATCH" | "POST" | "PUT";
  accessToken?: string;
  body?: unknown;
};

export type ApiTransportResponse<T> = {
  statusCode: number;
  data: ApiEnvelope<T>;
  headers: Record<string, unknown>;
  cookies: string[];
};

export interface ApiTransport {
  dispose?(): void;
  request<T>(request: ApiRequest): Promise<ApiTransportResponse<T>>;
}

export function buildApiUrl(baseUrl: string, path: string): string {
  return `${baseUrl.replace(/\/+$/u, "")}/${path.replace(/^\/+/, "")}`;
}

export function createTaroTransport(baseUrl: string): ApiTransport {
  const cookieBridge = new MemoryRefreshCookieBridge();
  return {
    dispose() {
      cookieBridge.clear();
    },
    async request<T>(request: ApiRequest): Promise<ApiTransportResponse<T>> {
      const header: Record<string, string> = { Accept: "application/json" };
      if (request.accessToken) {
        header.Authorization = `Bearer ${request.accessToken}`;
      }
      if (typeof request.body !== "undefined") {
        header["Content-Type"] = "application/json";
      }
      const cookieHeader = cookieBridge.cookieHeaderFor(request.path);
      if (cookieHeader) {
        header.Cookie = cookieHeader;
      }
      const response = await Taro.request<ApiEnvelope<T>>({
        url: buildApiUrl(baseUrl, request.path),
        method: request.method ?? "GET",
        header,
        data: request.body,
        dataType: "json",
        responseType: "text",
        timeout: 30_000,
      });
      cookieBridge.capture(response.cookies, response.header ?? {});
      return {
        statusCode: response.statusCode,
        data: response.data,
        headers: response.header ?? {},
        cookies: response.cookies ?? [],
      };
    },
  };
}
