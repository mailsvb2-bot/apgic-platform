import { SpecialistDirectory } from "./specialist-directory";

export const dynamic = "force-dynamic";
export const metadata = {
 title: "Каталог проверенных специалистов — APGIC",
 description: "Изучите опубликованные анкеты по направлениям работы. Допуск к консультации проверяется платформой.",
};
export default function SpecialistsPage(){
 return <main className="site-shell marketplace-shell">
  <header className="site-header">
   <a href="/" className="brand"><span className="brand-mark" aria-hidden="true">A</span> APGIC</a>
   <nav className="top-nav" aria-label="Навигация">
    <a href="/">Подбор по запросу</a><a href="/specialist">Стать специалистом</a>
   </nav>
  </header>
  <section className="marketplace-heading">
   <p className="eyebrow">APGIC · Marketplace</p>
   <h1>Выберите специалиста</h1>
   <p>В каталоге только опубликованные анкеты с подтверждёнными направлениями. Сравните опыт и подход, затем уточните свой запрос для записи.</p>
  </section>
  <SpecialistDirectory/>
 </main>;
}
