import { Journey } from "./journey";
import ResumeBooking from "../src/resume-booking";

// Public HTML must not be served with Next's immutable static-page cache.
export const dynamic = "force-dynamic";

const benefits = [
  "Без диагнозов по анкете",
  "Вы сами подтверждаете, как мы поняли запрос",
  "Оплата проходит у внешнего провайдера",
];

const directions = [
  "Тревога и стресс",
  "Сон",
  "Отношения",
  "Карьера и работа",
];

export default function Home() {
  return (
    <main>
      <div className="site-shell">
        <header className="site-header" aria-label="Главная навигация">
          <a className="brand" href="/" aria-label="APGIC — главная">
            <span className="brand-mark" aria-hidden="true">A</span>
            <span>APGIC</span>
          </a>
          <nav className="top-nav" aria-label="Разделы">
            <a href="#how">Как это работает</a>
            <a href="#start">Подобрать специалиста</a>
            <a className="organization-nav-link" href="/organization">Организация</a>
            <a className="pro-nav-link" href="/specialist">Для специалистов</a>
          </nav>
        </header>

        <section className="hero">
          <div className="hero-copy">
            <p className="eyebrow">Помощь, обучение и развитие — в одном месте</p>
            <h1>Найдите подходящего специалиста под ваш запрос</h1>
            <p className="hero-lead">
              Опишите своими словами, что сейчас важно. APGIC поможет уточнить запрос,
              покажет подходящих специалистов и проведёт до записи без лишней анкеты.
            </p>

            <div className="hero-actions">
              <a className="button-link primary-action" href="#start">
                Подобрать специалиста
              </a>
              <button className="secondary-action" type="button" disabled aria-describedby="avatar-status">
                Поговорить с аватаром
                <span className="coming-soon" id="avatar-status">скоро</span>
              </button>
            </div>

            <ul className="trust-list" aria-label="Что важно">
              {benefits.map((benefit) => <li key={benefit}>{benefit}</li>)}
            </ul>
          </div>

          <aside className="hero-card" aria-label="Как начинается подбор">
            <div className="hero-card-top">
              <span className="status-dot" aria-hidden="true" />
              <span>Начать можно без регистрации</span>
            </div>
            <h2>С чем можно обратиться</h2>
            <div className="direction-grid" aria-label="Популярные направления">
              {directions.map((direction) => <span key={direction}>{direction}</span>)}
            </div>
            <div className="mini-steps">
              <p><strong>1.</strong> Опишите ситуацию</p>
              <p><strong>2.</strong> Проверьте, как мы вас поняли</p>
              <p><strong>3.</strong> Выберите специалиста и время</p>
            </div>
          </aside>
        </section>

        <ResumeBooking />

        <section className="how-section" id="how" aria-labelledby="how-title">
          <div>
            <p className="section-kicker">Как это работает</p>
            <h2 id="how-title">Не каталог ради каталога, а понятный путь к следующему шагу</h2>
          </div>
          <div className="how-grid">
            <article>
              <span>01</span>
              <h4>Расскажите, что происходит</h4>
              <p>Пишите обычными словами. Не нужно заранее выбирать диагноз, профессию или услугу.</p>
            </article>
            <article>
              <span>02</span>
              <h4>Подтвердите смысл</h4>
              <p>Система предлагает темы, но окончательное подтверждение всегда остаётся за вами.</p>
            </article>
            <article>
              <span>03</span>
              <h4>Выберите человека и время</h4>
              <p>Показываются только подходящие варианты, после чего можно перейти к записи.</p>
            </article>
          </div>
        </section>

        <section className="start-section" id="start" aria-labelledby="start-title">
          <div className="start-heading">
            <p className="section-kicker">Подбор</p>
            <h2 id="start-title">Расскажите, что сейчас важно</h2>
            <p>
              Можно написать коротко или подробно. Сначала мы покажем, как поняли ваш запрос,
              и только после вашего подтверждения перейдём к подбору.
            </p>
          </div>
          <Journey />
        </section>

        <footer className="site-footer">
          <p>APGIC помогает с навигацией и подбором. Сервис не ставит диагнозы и не заменяет специалиста.</p>
        </footer>
      </div>
    </main>
  );
}
