import React, { useEffect, useState } from "react";

const fmt = (n: number) => {
  if (n >= 1e9) return (n/1e9).toFixed(2) + 'B';
  if (n >= 1e6) return (n/1e6).toFixed(2) + 'M';
  if (n >= 1e3) return (n/1e3).toFixed(1) + 'K';
  return n.toString();
};
const fmtCost = (n: number) => '$' + n.toFixed(2);

export default function App() {
  const [data, setData] = useState<{periods: any[], sources: any[], devices: string[]} | null>(null);
  const [selectedDevice, setSelectedDevice] = useState<string>("all");
  
  useEffect(() => {
    const fetchData = async () => {
      try {
        const res = await fetch("/api/stats?device=" + selectedDevice);
        const json = await res.json();
        setData(json);
      } catch (e) {
        console.error(e);
      }
    };
    fetchData();
    const interval = setInterval(fetchData, 5000);
    return () => clearInterval(interval);
  }, [selectedDevice]);

  if (!data) return <div className="min-h-screen bg-[#0a0f12] text-[#dee3e8] p-8 flex items-center justify-center font-['Inter']">Connecting to flight recorder...</div>;

  const { periods, sources } = data;

  return (
    <div className="bg-[#0a0f12] text-[#dee3e8] font-['Inter'] min-h-screen p-8 relative z-0">
      {/* Background Decoration Grid */}
      <div className="fixed inset-0 pointer-events-none opacity-[0.04] z-[-1]" style={{backgroundImage: "radial-gradient(#8ed5ff 1px, transparent 1px)", backgroundSize: "32px 32px"}}></div>

      {/* Header Section */}
      <header className="flex flex-col md:flex-row justify-between items-start md:items-center gap-4 mb-8 bg-[#111820]/40 p-4 rounded-xl border border-[#475569]/20">
        <div className="flex items-center gap-4">
          <h1 className="text-2xl font-bold tracking-tight flex items-center gap-2">
            <span className="material-symbols-outlined text-[#8ed5ff] text-3xl">rocket_launch</span>
            AI Flight Dashboard
          </h1>
          
          {data && (
            <select 
              value={selectedDevice} 
              onChange={e => setSelectedDevice(e.target.value)}
              className="bg-[#111820] text-[#8ed5ff] border border-[#475569]/40 rounded-md px-3 py-1 font-mono text-sm outline-none focus:border-[#8ed5ff] ml-4"
            >
              <option value="all">All Devices</option>
              {data.devices?.map((d: string) => (
                <option key={d} value={d}>{d}</option>
              ))}
            </select>
          )}

          <div className="flex items-center gap-2 bg-[#93000a]/10 px-3 py-1 border border-[#ffb4ab]/20 rounded-full ml-4">
            <span className="w-2 h-2 rounded-full bg-[#f87171] animate-pulse"></span>
            <span className="text-[#f87171] font-mono text-[11px] font-bold tracking-widest">LIVE_OPERATIONS</span>
          </div>
        </div>
      </header>

      {/* PeriodCost Stats Row */}
      <section className="grid grid-cols-2 md:grid-cols-4 lg:grid-cols-5 gap-4 mb-12">
        {periods.map((p: any, i: number) => (
          <div key={i} className={`bg-[#111820] border rounded-xl p-5 shadow-sm transition-all flex flex-col justify-between ${p.label === 'ALL' ? 'border-[#8ed5ff]/40 ring-1 ring-[#8ed5ff]/20' : 'border-[#475569]/30 hover:border-[#8ed5ff]/50'}`}>
            <div className="flex justify-between items-center mb-1">
              <span className="text-[#94a3b8] text-[10px] uppercase tracking-widest">{p.label}</span>
              <div className="flex gap-2 text-[8px] uppercase tracking-widest font-mono">
                <span className="text-[#38bdf8]" title="Base In">IN:{fmt(Math.max(0, p.input_tokens - p.cached_tokens))}</span>
                <span className="text-[#ffc176]" title="Cached">CA:{fmt(p.cached_tokens)}</span>
                <span className="text-[#bcc7de]" title="Output">OUT:{fmt(p.output_tokens)}</span>
              </div>
            </div>
            <div className={`text-3xl font-mono mt-1 ${p.label === 'ALL' ? 'text-[#8ed5ff]' : 'text-[#dee3e8]'}`}>{fmtCost(p.cost)}</div>
          </div>
        ))}
      </section>

      {/* Source Stats Grid */}
      <section className="grid grid-cols-1 xl:grid-cols-2 gap-8">
        {sources.map((src: any, si: number) => {
           const isClaude = src.name.includes('Claude') || src.name.includes('Anthropic');
           const accentColor = isClaude ? '#f1a02b' : '#38bdf8'; // Primary or tertiary
           const accentClass = isClaude ? 'text-[#f1a02b]' : 'text-[#38bdf8]';
           const bgClass = isClaude ? 'bg-[#f1a02b]/10' : 'bg-[#38bdf8]/10';

           const baseInput = Math.max(0, src.total_input - src.total_cached);
           const totalTokens = baseInput + src.total_cached + src.total_output;
           
           const inPct = totalTokens > 0 ? (baseInput / totalTokens) * 100 : 0;
           const outPct = totalTokens > 0 ? (src.total_output / totalTokens) * 100 : 0;
           const cachedPct = totalTokens > 0 ? (src.total_cached / totalTokens) * 100 : 0;

           const formatPct = (pct: number) => {
             if (pct > 0 && pct < 1) return '<1%';
             return pct.toFixed(0) + '%';
           };

           const sortedModels = [...src.models].sort((a: any, b: any) => b.total_cost - a.total_cost);

           return (
            <article key={si} className="bg-[#111820] border border-[#475569]/30 rounded-2xl overflow-hidden shadow-lg flex flex-col">
              <div className="p-6 bg-[#1b2024]/20 border-b border-[#475569]/30 flex justify-between items-center">
                <div>
                  <h2 className="text-xl font-bold text-[#dee3e8] flex items-center gap-3">
                    <span className={`material-symbols-outlined ${accentClass} ${bgClass} p-1.5 rounded-lg`}>{isClaude ? 'memory' : 'hub'}</span>
                    {src.name}
                  </h2>
                </div>
                <div className="text-right">
                  <span className="text-[10px] text-[#94a3b8] uppercase tracking-widest font-mono">Total Spend</span>
                  <div className={`text-3xl font-mono mt-0.5 ${accentClass}`}>{fmtCost(src.total_cost)}</div>
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 gap-0 border-b border-[#475569]/20">
                {/* Token Distribution Grid */}
                <div className="p-6 border-r border-[#475569]/20 grid grid-cols-2 gap-6 bg-[#111820]">
                  <div className="border-l-2 border-[#38bdf8] pl-4 py-1">
                    <span className="text-[9px] uppercase text-[#94a3b8] tracking-wider font-mono">Base In</span>
                    <div className="text-xl font-bold font-mono">{fmt(baseInput)}</div>
                  </div>
                  <div className="border-l-2 border-[#ffc176] pl-4 py-1">
                    <span className="text-[9px] uppercase text-[#94a3b8] tracking-wider font-mono">Cached</span>
                    <div className="text-xl font-bold font-mono">{fmt(src.total_cached)}</div>
                  </div>
                  <div className="border-l-2 border-[#bcc7de] pl-4 py-1">
                    <span className="text-[9px] uppercase text-[#94a3b8] tracking-wider font-mono">Output</span>
                    <div className="text-xl font-bold font-mono">{fmt(src.total_output)}</div>
                  </div>
                  <div className="border-l-2 border-[#8ed5ff] pl-4 py-1">
                    <span className="text-[9px] uppercase text-[#94a3b8] tracking-wider font-mono">Aggregate</span>
                    <div className="text-xl font-bold font-mono">{fmt(totalTokens)}</div>
                  </div>
                </div>

                {/* Donut Chart Area */}
                <div className="p-6 flex items-center justify-center gap-8 bg-[#111820]">
                  <div className="relative w-20 h-20">
                    <svg className="w-full h-full transform -rotate-90" viewBox="0 0 36 36">
                      <circle cx="18" cy="18" fill="none" r="16" stroke="#1e293b" strokeWidth="3"></circle>
                      <circle cx="18" cy="18" fill="none" r="16" stroke="#38bdf8" strokeDasharray={`${inPct} 100`} strokeWidth="3"></circle>
                      <circle cx="18" cy="18" fill="none" r="16" stroke="#ffc176" strokeDasharray={`${cachedPct} 100`} strokeDashoffset={`-${inPct}`} strokeWidth="3"></circle>
                      <circle cx="18" cy="18" fill="none" r="16" stroke="#bcc7de" strokeDasharray={`${outPct} 100`} strokeDashoffset={`-${inPct + cachedPct}`} strokeWidth="3"></circle>
                    </svg>
                  </div>
                  <div className="flex flex-col gap-1.5">
                    <div className="flex items-center gap-2 text-[9px] font-mono">
                      <span className="w-1.5 h-1.5 bg-[#38bdf8] rounded-sm"></span> IN ({formatPct(inPct)})
                    </div>
                    <div className="flex items-center gap-2 text-[9px] font-mono">
                      <span className="w-1.5 h-1.5 bg-[#ffc176] rounded-sm"></span> CACHED ({formatPct(cachedPct)})
                    </div>
                    <div className="flex items-center gap-2 text-[9px] font-mono">
                      <span className="w-1.5 h-1.5 bg-[#bcc7de] rounded-sm"></span> OUT ({formatPct(outPct)})
                    </div>
                  </div>
                </div>
              </div>

              {/* Model Table */}
              <div className="overflow-x-auto bg-[#05070a]">
                <table className="w-full text-left font-mono">
                  <thead>
                    <tr className="bg-[#1b2024]/40 border-b border-[#475569]/30">
                      <th className="px-6 py-3 text-[9px] uppercase tracking-widest text-[#94a3b8]">Model Identifier</th>
                      <th className="px-6 py-3 text-[9px] uppercase tracking-widest text-[#94a3b8]">Rates (Per 1M)</th>
                      <th className="px-6 py-3 text-[9px] uppercase tracking-widest text-[#94a3b8]">Events</th>
                      <th className="px-6 py-3 text-[9px] uppercase tracking-widest text-[#94a3b8] text-right">Subtotal</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-[#475569]/10">
                    {sortedModels.map((m: any, mi: number) => (
                      <tr key={mi} className="hover:bg-[#8ed5ff]/5 transition-colors">
                        <td className={`px-6 py-3.5 ${accentClass}`}>{m.model}</td>
                        <td className="px-6 py-3.5 text-[10px] text-[#94a3b8]">
                          In: {fmtCost(m.input_price_per_m || 0)} | Ca: {fmtCost(m.cached_price_per_m || 0)} | Out: {fmtCost(m.output_price_per_m || 0)}
                        </td>
                        <td className="px-6 py-3.5">{m.events}</td>
                        <td className="px-6 py-3.5 text-right font-bold">{fmtCost(m.total_cost)}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </article>
           );
        })}
      </section>
    </div>
  );
}
