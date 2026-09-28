export const metadata = {
  title: "Для специалистов — APGIC",
  description: "Профессиональный вход APGIC: профиль специалиста, подтверждение компетенций, проверка и публикация.",
};

const steps = [
  {
    number: "01",
    title: "Единая Identity",
    text: "Если вы уже пользовались APGIC как клиент, новый аккаунт не нужен. Роль специалиста добавляется к той же Identity.",
  },
  {
    number: "02",
    title: "Профиль и компетенции",
    text: "Вы заполняете профессиональный профиль и заявляете направления работы. Само заявление ещё не означает, что компетенция проверена APGIC.",
  },
  {
    number: "03",
    title: "Документы и проверка",
    text: "Подтверждающие материалы проходят intake и review. До завершения проверки профиль не получает статус APGIC VERIFIED.",
  },
  {
    number: "04",
    title: "Квалификация и публикация",
    text: "Публичная видимость включается только после обязательных проверок и QualificationPolicy. Профиль можно будет публиковать и снимать с публикации.",
  },
];

export default function SpecialistPage() {
  return (
    <main>
      <div className="site-shell specialist-shell">
        <header className="site-header" aria-label="Профессиональная навигация">
          <a className="brand" href="/" aria-label="APGIC — главная">
            <span className="brand-mark" aria-hidden="true">A</span>
            <span>APGIC</span>
          </a>
          <nav className="top-nav" aria-label="Разделы">
            <a href="/">Для клиентов</a>
            <a href="#path">Как подключиться</a>
          </nav>
        </header>

        <section className="specialist-hero">
          <div>
            <p className="eyebrow">APGIC · Для специалистов</p>
            <h1>Работайте с клиентами через единый профессиональный профиль</h1>
            <p className="hero-lead">
              Профессиональный контур APGIC строится вокруг проверяемого профиля: заявленные компетенции,
              подтверждающие материалы, review и управляемая публикация в marketplace.
            </p>
            <div className="hero-actions">
              <a className="button-link primary-action" href="#path">Посмотреть этапы подключения</a>
              <a className="button-link secondary-action" href="/">Вернуться к клиентскому входу</a>
            </div>
          </div>

          <aside className="hero-card specialist-status-card" aria-label="Статус профессионального подключения">
            <p className="section-kicker">Профессиональный контур</p>
            <h2>Подключение идёт по проверяемым этапам</h2>
            <ul className="status-list">
              <li><span className="status-dot neutral-dot" aria-hidden="true" /><span>Identity и роль специалиста</span></li>
              <li><span className="status-dot neutral-dot" aria-hidden="true" /><span>Evidence intake и review</span></li>
              <li><span className="status-dot neutral-dot" aria-hidden="true" /><span>Qualification и публикация</span></li>
            </ul>
            <p className="status-note">
              Текущий статус этапов здесь не определяется: страница не знает, авторизован ли посетитель и какие шаги он уже прошёл.
              Самостоятельная отправка профиля из Web будет включена только вместе с серверным evidence intake.
            </p>
          </aside>
        </section>

        <section className="how-section" id="path" aria-labelledby="specialist-path-title">
          <div>
            <p className="section-kicker">Supply Path</p>
            <h2 id="specialist-path-title">Как специалист появляется в APGIC</h2>
          </div>
          <div className="how-grid specialist-grid">
            {steps.map((step) => (
              <article key={step.number}>
                <span>{step.number}</span>
                <h4>{step.title}</h4>
                <p>{step.text}</p>
              </article>
            ))}
          </div>
        </section>

        <section className="professional-boundary" aria-labelledby="professional-boundary-title">
          <div>
            <p className="section-kicker">Прозрачность статусов</p>
            <h2 id="professional-boundary-title">Заявлено ≠ подтверждено</h2>
          </div>
          <div className="boundary-copy">
            <p>
              До завершения проверки профессиональное направление остаётся заявленным или документально
              поддержанным. Статус <strong>APGIC VERIFIED</strong> присваивается только конкретной проверенной компетенции
              или направлению после предусмотренного review.
            </p>
            <p>
              Неполный профиль или незавершённый review не может стать публично доступным в подборе.
            </p>
          </div>
        </section>

        <footer className="site-footer">
          <p>APGIC использует одну Identity для разных ролей пользователя. Отдельный аккаунт специалиста не создаётся.</p>
        </footer>
      </div>
    </main>
  );
}
