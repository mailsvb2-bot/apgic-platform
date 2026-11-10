import type {Metadata} from "next";
import {GrowthWorkspace} from "./workspace";
export const dynamic="force-dynamic";
export const metadata:Metadata={title:"Привлечение клиентов — APGIC",robots:{index:false,follow:false}};
export default function Page(){
 return <main className="site-shell">
  <header className="site-header"><a className="brand" href="/specialist">APGIC · Специалист</a></header>
  <section className="start-section"><div className="start-heading">
   <p className="section-kicker">Growth & Distribution</p>
   <h1>Привлечение клиентов</h1>
   <p>Создавайте ссылки только на ваши действующие предложения и отслеживайте реальные клиентские запросы.</p>
  </div><GrowthWorkspace/></section>
 </main>;
}
