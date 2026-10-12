import { AvatarWorkspace } from "./workspace";

export const dynamic = "force-dynamic";
export default function AvatarPage() {
  return (
    <main className="site-shell" style={{ maxWidth: 860, margin: "0 auto", padding: "2rem 1rem 5rem" }}>
      <header className="site-header"><a className="brand" href="/">APGIC</a><a href="/#start">Выбрать живого специалиста</a></header>
      <h1>Поговорить с AI-аватаром</h1>
      <p><strong>Это виртуальный собеседник, не человек и не специалист.</strong> Он не ставит диагнозы,
        не проводит терапию и не заменяет экстренную или профессиональную помощь.</p>
      <AvatarWorkspace />
    </main>
  );
}
