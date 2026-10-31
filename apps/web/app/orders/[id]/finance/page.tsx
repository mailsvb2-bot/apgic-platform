import FinanceStatusView from "../../../../src/finance-status";

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <FinanceStatusView orderID={id} />;
}
