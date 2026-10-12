import type { MetadataRoute } from "next";

// Public search inventory comes from the canonical, qualification-backed APGIC
// business owner, not a second copy of the Demand Ontology in the web app.
export const dynamic = "force-dynamic";

type IndexPage = { slug: string };
type SearchInventory = {
  pages?: IndexPage[];
  publication_policy_version?: string;
};

export default async function sitemap(): Promise<MetadataRoute.Sitemap> {
  const root = "https://apgic.ru";
  const origin = process.env.APGIC_API_ORIGIN ?? "http://127.0.0.1:43111";

  try {
    const response = await fetch(new URL("/v1/public/help", origin), {
      cache: "no-store",
      headers: { Accept: "application/json" },
    });
    if (!response.ok) return [{ url: root }];

    const inventory = (await response.json()) as SearchInventory;
    if (
      !Array.isArray(inventory.pages) ||
      inventory.publication_policy_version !== "search-publication-v1-qualified-supply"
    ) {
      return [{ url: root }];
    }

    const slugs = new Set<string>();
    for (const page of inventory.pages) {
      if (typeof page?.slug !== "string" || !/^[a-z]{2,24}$/.test(page.slug)) {
        return [{ url: root }];
      }
      slugs.add(page.slug);
    }

    return [
      { url: root },
      ...[...slugs].sort().map((slug) => ({ url: `${root}/help/${slug}` })),
    ];
  } catch {
    // Fail closed on search/index outages. Never submit unqualified pages.
    return [{ url: root }];
  }
}
