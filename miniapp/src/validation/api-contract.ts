import type { AuthLoginRequest, AuthLoginResponse } from "@deeix/api-contract";

export type LoginCredentials = Pick<AuthLoginRequest, "username" | "password">;

export function createLoginRequest(credentials: LoginCredentials): AuthLoginRequest {
  return {
    username: credentials.username,
    password: credentials.password,
  };
}

export function hasUsableAccessToken(result: Pick<AuthLoginResponse, "accessToken">): boolean {
  return result.accessToken.trim().length > 0;
}
