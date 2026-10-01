import { appleAppSiteAssociationFromEnv } from "../../../src/mobile-app-link-association";

export const dynamic = "force-dynamic";

export function GET() {
  try {
    return Response.json(appleAppSiteAssociationFromEnv(), {
      headers: {
        "cache-control": "public, max-age=300",
        "content-type": "application/json",
      },
    });
  } catch {
    return Response.json(
      { code: "APP_LINK_ASSOCIATION_NOT_CONFIGURED" },
      { status: 503, headers: { "cache-control": "no-store" } },
    );
  }
}
