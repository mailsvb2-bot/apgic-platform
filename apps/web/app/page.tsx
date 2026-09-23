const surfaces = ["Web", "PWA", "iOS", "Android"];

export default function Home() {
  return (
    <main className="shell">
      <section className="hero" aria-labelledby="title">
        <p className="eyebrow">R0 · Foundation</p>
        <h1 id="title">APGIC Platform</h1>
        <p>
          Единая business truth, provider-neutral connectors и одинаковые contracts
          для всех клиентских поверхностей.
        </p>
        <ul aria-label="Поддерживаемые поверхности">
          {surfaces.map((surface) => <li key={surface}>{surface}</li>)}
        </ul>
      </section>
    </main>
  );
}
