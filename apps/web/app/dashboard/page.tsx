const services = [
  ["API", "Ready", "Port 8080"],
  ["Worker", "Ready", "Queue connected"],
  ["PostgreSQL", "Ready", "Desired state"],
  ["Redis", "Ready", "Jobs and events"],
];

export default function DashboardPage() {
  return (
    <main className="mx-auto min-h-screen max-w-6xl px-8 py-12">
      <div className="flex items-end justify-between border-b border-zinc-800 pb-8">
        <div>
          <p className="text-sm font-medium text-emerald-400">Local development</p>
          <h1 className="mt-2 text-4xl font-semibold">Ship dashboard</h1>
        </div>
        <span className="rounded-full border border-emerald-900 bg-emerald-950 px-3 py-1 text-sm text-emerald-300">Foundation ready</span>
      </div>
      <section className="grid gap-4 py-8 sm:grid-cols-2">
        {services.map(([name, status, detail]) => (
          <article className="rounded-xl border border-zinc-800 bg-zinc-900 p-6" key={name}>
            <div className="flex items-center justify-between">
              <h2 className="font-semibold">{name}</h2>
              <span className="text-sm text-emerald-400">{status}</span>
            </div>
            <p className="mt-4 text-sm text-zinc-500">{detail}</p>
          </article>
        ))}
      </section>
    </main>
  );
}
