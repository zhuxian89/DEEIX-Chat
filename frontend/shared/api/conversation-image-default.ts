import { authedRequest } from "@/shared/api/authed-client";
import type { ConversationDefaultModelCandidateResponse } from "@deeix/api-contract";

export function getConversationDefaultImageModel(accessToken: string) {
  return authedRequest<ConversationDefaultModelCandidateResponse>(
    "/api/v1/conversations/default-image-model-candidate",
    { accessToken },
    true,
  );
}
