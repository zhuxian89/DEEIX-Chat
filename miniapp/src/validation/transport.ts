import type {
  ApiEnvelope,
  ApiRequest,
  ApiTransport,
  ApiTransportResponse,
} from "@/platform/transport";

export { buildApiUrl, createTaroTransport } from "@/platform/transport";
export type { ApiEnvelope };
export type ProbeRequest = ApiRequest;
export type ProbeTransport = ApiTransport;
export type ProbeTransportResponse<T> = ApiTransportResponse<T>;
