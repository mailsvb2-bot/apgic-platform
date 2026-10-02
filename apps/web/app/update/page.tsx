// Public HTML must not be served with Next's immutable static-page cache.
export const dynamic = "force-dynamic";

export default function UpdatePage() {
  return (
    <main>
      <div className="site-shell">
        <header className="site-header" aria-label="Главная навигация">
          <a className="brand" href="/" aria-label="APGIC — главная">
            <span className="brand-mark" aria-hidden="true">A</span>
            <span>APGIC</span>
          </a>
        </header>

        <section className="specialist-hero" aria-labelledby="update-title">
          <div>
            <p className="eyebrow">Безопасное обновление</p>
            <h1 id="update-title">Обновите приложение APGIC</h1>
            <p className="hero-lead">
              Эта страница открывается, когда установленная версия приложения больше не может
              безопасно работать с текущим сервером.
            </p>
            <div className="hero-actions">
              <a className="button-link primary-action" href="/">
                Вернуться на сайт
              </a>
            </div>
          </div>

          <aside className="hero-card" aria-label="Как обновить приложение">
            <div className="hero-card-top">
              <span className="status-dot" aria-hidden="true" />
              <span>Ваши данные не нужно создавать заново</span>
            </div>
            <h2>Что делать</h2>
            <div className="mini-steps">
              <p><strong>1.</strong> Откройте тот официальный магазин или канал, через который установили APGIC.</p>
              <p><strong>2.</strong> Установите доступное обновление поверх текущей версии.</p>
              <p><strong>3.</strong> Снова откройте приложение и продолжите работу.</p>
            </div>
            <p className="status-note">
              Если обновление пока не предлагается, попробуйте позже. Не удаляйте приложение и
              не повторяйте оплату или уже отправленное действие только из-за запроса на обновление.
            </p>
          </aside>
        </section>
      </div>
    </main>
  );
}
