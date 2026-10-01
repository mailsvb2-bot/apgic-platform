import DeepLinkRedirect from "./DeepLinkRedirect";

export default async function DeepLinkPage({
  params,
}: {
  params: Promise<{ token: string }>;
}) {
  const { token } = await params;
  return <DeepLinkRedirect token={token} />;
}
