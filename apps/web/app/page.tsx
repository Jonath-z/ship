import Link from "next/link";

export default function Home() {
  return (
    <main className="mx-auto flex min-h-screen max-w-5xl flex-col justify-center px-8 py-16">
      <p className="mb-4 text-sm font-semibold uppercase tracking-[0.28em] text-emerald-400">Ship control plane</p>
      <h1 className="max-w-3xl text-5xl font-semibold tracking-tight sm:text-7xl">Your servers. Your apps. One simple control plane.</h1>
      <p className="mt-6 max-w-2xl text-lg leading-8 text-zinc-400">Configure and operate Kamal deployments without putting infrastructure configuration in your application repository.</p>
      <div className="mt-10">
        <Link className="rounded-lg bg-emerald-400 px-5 py-3 font-semibold text-zinc-950 hover:bg-emerald-300" href="/dashboard">Open dashboard</Link>
      </div>
    </main>
  );
}
