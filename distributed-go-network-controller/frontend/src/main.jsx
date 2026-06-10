import React from 'react';
import { createRoot } from 'react-dom/client';
import './styles.css';

const cards = [
  {
    title: 'Deployments',
    description: 'Track automation rollout status across network environments.',
  },
  {
    title: 'Workers',
    description: 'Monitor distributed agents that execute infrastructure jobs.',
  },
  {
    title: 'Devices',
    description: 'View managed network inventory and device connectivity.',
  },
  {
    title: 'Drift Detection',
    description: 'Surface configuration drift before it becomes operational risk.',
  },
];

function App() {
  return (
    <main className="app-shell">
      <section className="overview">
        <p className="eyebrow">Phase 1 Scaffold</p>
        <h1>Distributed Network Automation Controller</h1>
      </section>

      <section className="card-grid" aria-label="Controller capabilities">
        {cards.map((card) => (
          <article className="feature-card" key={card.title}>
            <h2>{card.title}</h2>
            <p>{card.description}</p>
          </article>
        ))}
      </section>
    </main>
  );
}

createRoot(document.getElementById('root')).render(<App />);
