import { Journey } from "./journey";

const surfaces = ["Web", "PWA", "iOS", "Android"];

export default function Home() {
  return (
    <main className="shell">
      <header className="hero">
        <p className="eyebrow">APGIC · клиентский вход</p>
        <h1 id="title">Опишите запрос — и проверьте, как мы его поняли</h1>
        <p>
          Свободный текст становится HelpIntent. Вы подтверждаете темы сами.
          Подбор показывает только допущенных специалистов, а слот удерживается эксклюзивно.
        </p>
        <ul aria-label="Поддерживаемые поверхности">
          {surfaces.map((surface) => <li key={surface}>{surface}</li>)}
        </ul>
      </header>
      <Journey />
    </main>
  );
}
