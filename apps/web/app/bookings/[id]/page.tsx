import BookingDetail from "../../../src/booking-detail";

export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <BookingDetail bookingID={id} />;
}
