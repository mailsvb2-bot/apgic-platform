import { DutyConsole } from "./duty-console";
export const dynamic = "force-dynamic";
export const metadata={title:"Дежурство — кабинет специалиста APGIC"};
export default function DutyPage(){
 return <main className="site-shell" style={{maxWidth:900,margin:"auto",padding:"2rem 1rem"}}>
  <nav><a href="/specialist">← Кабинет специалиста</a></nav>
  <h1>Принимать сейчас</h1>
  <p>Дежурство доступно только проверенному специалисту с действующим опубликованным платным предложением. Это не включает оплату или видеосвязь автоматически.</p>
  <DutyConsole/>
 </main>;
}