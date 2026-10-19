import ClientPersonalView from "../../src/client-personal-view";

export const dynamic="force-dynamic";
export const metadata={title:"Мой кабинет — APGIC",description:"Ваши записи, история встреч и персональные настройки в APGIC."};
export default function PersonalPage(){
  return <main className="personal-shell">
    <header className="site-header" aria-label="Навигация личного кабинета">
      <a className="brand" href="/"><span className="brand-mark" aria-hidden="true">A</span><span>APGIC</span></a>
      <nav className="top-nav" aria-label="Разделы"><a href="/">Главная</a><a href="/#start">Подобрать специалиста</a></nav>
    </header>
    <div className="personal-heading"><p className="eyebrow">APGIC Personal</p><h1>Мой кабинет</h1>
      <p>Ваши встречи, подтверждённая история и настройки — в одном месте.</p></div>
    <ClientPersonalView />
  </main>;
}
