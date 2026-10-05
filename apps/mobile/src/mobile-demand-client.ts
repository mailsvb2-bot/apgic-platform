import type {
  ConfirmHelpIntentRequest,
  CreateHelpIntentRequest,
  ErrorEnvelope,
  HelpIntent,
} from "../../../packages/contracts/src/generated/apgic-v1.ts";

export type MobileDemandClientOptions = {
  baseURL: string;
  sessionCookie?: string;
};

function endpoint(baseURL: string, path: string): string {
  return `${baseURL.replace(/\/$/, "")}${path}`;
}

function headers(options: MobileDemandClientOptions): Record<string, string> {
  const result: Record<string, string> = {
    "content-type": "application/json",
  };
  if (options.sessionCookie) {
    result.cookie = options.sessionCookie;
  }
  return result;
}

async function readError(response: Response): Promise<Error> {
  try {
    const body = (await response.json()) as Partial<ErrorEnvelope>;
    return new Error(body.message_safe || body.code || `HTTP_${response.status}`);
  } catch {
    return new Error(`HTTP_${response.status}`);
  }
}

export async function createMobileHelpIntent(
  freeText: string,
  options: MobileDemandClientOptions,
): Promise<HelpIntent> {
  const body: CreateHelpIntentRequest = {free_text: freeText};
  const response = await fetch(endpoint(options.baseURL, "/v1/help-intents"), {
    method: "POST",
    headers: headers(options),
    credentials: "include",
    body: JSON.stringify(body),
  });
  if (!response.ok) {
    throw await readError(response);
  }
  const intent = (await response.json()) as HelpIntent;
  assertHelpIntentSafety(intent, "DRAFT");
  return intent;
}

export async function confirmMobileHelpIntent(
  intentID: string,
  request: ConfirmHelpIntentRequest,
  options: MobileDemandClientOptions,
): Promise<HelpIntent> {
  const response = await fetch(
    endpoint(options.baseURL, `/v1/help-intents/${encodeURIComponent(intentID)}/confirm`),
    {
      method: "POST",
      headers: headers(options),
      credentials: "include",
      body: JSON.stringify(request),
    },
  );
  if (!response.ok) {
    throw await readError(response);
  }
  const intent = (await response.json()) as HelpIntent;
  assertHelpIntentSafety(intent, "CONFIRMED");
  return intent;
}

export function assertHelpIntentSafety(
  intent: HelpIntent,
  expectedStatus?: "DRAFT" | "CONFIRMED",
): void {
  if (!intent.id || !intent.client_identity_id || !intent.free_text) {
    throw new Error("HELP_INTENT_IDENTITY_OR_TEXT_MISSING");
  }
  if (intent.diagnosis_asserted !== false) {
    throw new Error("HELP_INTENT_DIAGNOSIS_ASSERTED");
  }
  if (!intent.notice || !Array.isArray(intent.topics) || !Array.isArray(intent.goals)) {
    throw new Error("HELP_INTENT_INTERPRETATION_INVALID");
  }
  if (expectedStatus && intent.status !== expectedStatus) {
    throw new Error(`HELP_INTENT_STATUS_${intent.status || "MISSING"}`);
  }
}

export type MobileDemandE2EResult = {
  identityID: string;
  intentID: string;
  originalTopics: string[];
  confirmedTopics: string[];
  diagnosisAsserted: false;
  userCorrectionObserved: true;
};

export async function runMobileDemandE2E(options: {
  baseURL: string;
  sessionCookie: string;
  freeText: string;
  correctedTopics: string[];
}): Promise<MobileDemandE2EResult> {
  const draft = await createMobileHelpIntent(options.freeText, options);
  if (options.correctedTopics.length === 0) {
    throw new Error("HELP_INTENT_E2E_CORRECTION_REQUIRED");
  }
  const originalTopics = [...draft.topics];
  const sameTopics =
    originalTopics.length === options.correctedTopics.length &&
    originalTopics.every((topic, index) => topic === options.correctedTopics[index]);
  if (sameTopics) {
    throw new Error("HELP_INTENT_E2E_CORRECTION_NOT_OBSERVED");
  }
  const confirmed = await confirmMobileHelpIntent(
    draft.id,
    {
      topics: options.correctedTopics,
      goals: draft.goals,
      context: draft.context,
    },
    options,
  );
  if (confirmed.client_identity_id !== draft.client_identity_id) {
    throw new Error("HELP_INTENT_IDENTITY_CHANGED");
  }
  if (
    confirmed.topics.length !== options.correctedTopics.length ||
    confirmed.topics.some((topic, index) => topic !== options.correctedTopics[index])
  ) {
    throw new Error("HELP_INTENT_USER_CORRECTION_NOT_PRESERVED");
  }
  return {
    identityID: confirmed.client_identity_id,
    intentID: confirmed.id,
    originalTopics,
    confirmedTopics: [...confirmed.topics],
    diagnosisAsserted: false,
    userCorrectionObserved: true,
  };
}
