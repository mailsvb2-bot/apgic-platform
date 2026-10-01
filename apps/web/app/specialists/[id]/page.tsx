import DeepLinkedResource from "../../../src/deep-linked-resource";

export default async function Page({
  params,
  searchParams,
}: {
  params: Promise<{ id: string }>;
  searchParams: Promise<{ link?: string }>;
}) {
  const [{ id }, { link = "" }] = await Promise.all([params, searchParams]);
  return <DeepLinkedResource kind="SPECIALIST" id={id} token={link} />;
}
