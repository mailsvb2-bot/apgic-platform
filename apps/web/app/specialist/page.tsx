import {SpecialistDashboard} from "./specialist-dashboard";

export const dynamic = "force-dynamic";
export const metadata = {
 title: "Кабинет специалиста — APGIC",
 description: "Профиль, подтверждение компетенций, услуги, расписание и консультации специалиста APGIC."
};

export default function SpecialistPage(){
 return <SpecialistDashboard/>;
}
