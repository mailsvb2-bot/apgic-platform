import ConsultationJourney from "../../../../src/consultation-journey";
import ConsultationResultView from "../../../../src/consultation-result";
import ConsultationHistory from "../../../../src/consultation-history";

// This page contains no specialist-secret, media grant or local booking truth.
// Every readiness, room issuance and result call independently authenticates
// the specialist's signed APGIC session and canonical booking ownership.
export default async function Page({params}:{params:Promise<{id:string}>}) {
  const {id}=await params;
  return <main>
    <h1>Моя консультация — рабочее место специалиста</h1>
    <p>Откройте запись по её идентификатору. Перед входом сервис проверит вашу роль специалиста, подтверждение записи, действующее право и готовность обеих сторон.</p>
    <ConsultationJourney bookingID={id}/>
    <ConsultationResultView bookingID={id}/>
    <ConsultationHistory bookingID={id}/>
    <p><a href="/specialist">Вернуться в кабинет специалиста</a></p>
  </main>;
}
