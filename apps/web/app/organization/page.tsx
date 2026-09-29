import { OrganizationWorkspace } from "./workspace";

export const dynamic = "force-dynamic";

export const metadata = {
  title: "Организация — APGIC",
  description: "Рабочее пространство владельца организации: направления и их жизненный цикл.",
};

export default function OrganizationPage() {
  return (
    <main>
      <div className="site-shell organization-shell">
        <header className="site-header" aria-label="Организационная навигация">
          <a className="brand" href="/" aria-label="APGIC — главная">
            <span className="brand-mark" aria-hidden="true">A</span>
            <span>APGIC</span>
          </a>
          <nav className="top-nav" aria-label="Разделы">
            <a href="/">Для клиентов</a>
            <a href="/specialist">Для специалистов</a>
          </nav>
        </header>

        <section className="organization-hero">
          <div>
            <p className="eyebrow">APGIC · Организации</p>
            <h1>Управляйте организацией и её направлениями</h1>
            <p className="hero-lead">
              Организация хранит свою бизнес-историю в APGIC. Направления можно создавать и архивировать,
              но не удалять задним числом вместе с зависимыми данными.
            </p>
          </div>
          <aside className="hero-card">
            <p className="section-kicker">Что уже работает</p>
            <h2>Жизненный цикл организации</h2>
            <div className="mini-steps">
              <p><strong>1.</strong> Создайте организацию</p>
              <p><strong>2.</strong> Добавьте направления</p>
              <p><strong>3.</strong> Архивируйте завершённые направления без потери истории</p>
            </div>
          </aside>
        </section>

        <OrganizationWorkspace />

        <footer className="site-footer">
          <p>Изменения организации сохраняются в APGIC и привязаны к вашей текущей Identity.</p>
        </footer>
      </div>
    </main>
  );
}
