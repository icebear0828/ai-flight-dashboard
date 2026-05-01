import React, { useEffect, useState } from "react";
import SettingsModal from "./SettingsModal";
import Radar from "./components/Radar";

const fmt = (n: number) => {
  if (n >= 1e9) return (n/1e9).toFixed(2) + 'B';
  if (n >= 1e6) return (n/1e6).toFixed(2) + 'M';
  if (n >= 1e3) return (n/1e3).toFixed(1) + 'K';
  return n.toString();
};
const fmtCost = (n: number) => '$' + n.toFixed(2);

export default function App() {
  const [data, setData] = useState<{periods: any[], sources: any[], devices: any[]} | null>(null);
  const [selectedDevice, setSelectedDevice] = useState<string>("all");
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string>("");
  
  useEffect(() => {
    const fetchData = async () => {
      try {
        // Works in both Wails (assets handler) and standalone Web mode
        const res = await fetch("/api/stats?device=" + selectedDevice);
        if (!res.ok) {
           throw new Error(`HTTP ${res.status}: ${await res.text()}`);
        }
        const json = await res.json();
        setData(json);
        setErrorMsg("");
      } catch (e: any) {
        console.error(e);
        setErrorMsg(e.toString());
      }
    };
    fetchData();
    const interval = setInterval(fetchData, 5000);
    return () => clearInterval(interval);
  }, [selectedDevice]);

  if (errorMsg) return (
    <div className="min-h-screen bg-bg-deep text-red-500 p-[40px] flex items-center justify-center font-display text-[24px] uppercase tracking-widest relative overflow-hidden">
      <div className="z-10 bg-black/50 p-8 border-2 border-red-500 shadow-[0_0_20px_rgba(255,0,0,0.5)]">
        ERROR FETCHING DATA: <br/> {errorMsg}
      </div>
    </div>
  );

  if (!data) return (
    <div className="min-h-screen bg-bg-deep text-neon-cyan flex flex-col items-center justify-center font-display text-[24px] uppercase tracking-widest relative overflow-hidden">
      <div className="absolute inset-0 bg-[radial-gradient(ellipse_at_center,_var(--tw-gradient-stops))] from-neon-cyan/20 via-bg-deep to-bg-deep"></div>
      <div className="z-10 animate-pulse">SYSTEM INITIALIZING...</div>
    </div>
  );

  const { periods = [], sources = [] } = data;

  return (
    <div className="min-h-screen p-[24px] md:p-[40px]">
      
      {/* Header Section */}
      <header className="wails-drag flex flex-col md:flex-row justify-between items-start md:items-end gap-[24px] mb-[40px] pb-[24px] border-b border-panel-border">
        <div>
          <h1 className="font-display text-[48px] md:text-[64px] leading-[1.0] font-bold tracking-tight mb-[16px] bg-clip-text text-transparent bg-gradient-to-r from-white to-gray-400">
            Aurora AI
          </h1>
          <div className="flex flex-col md:flex-row md:items-center gap-[16px]">
            <div className="glass-panel px-[16px] py-[6px] flex items-center gap-[8px] border-neon-cyan/50 shadow-[0_0_15px_rgba(0,240,255,0.2)]">
              <span className="w-2 h-2 rounded-full bg-neon-cyan animate-pulse"></span>
              <span className="font-sans text-[11px] font-semibold text-white tracking-[1px]">LIVE_OPERATIONS</span>
            </div>
            {data && (
              <select 
                value={selectedDevice} 
                onChange={e => setSelectedDevice(e.target.value)}
                className="glass-panel bg-transparent text-white px-[16px] py-[6px] font-mono text-[14px] outline-none cursor-pointer hover:border-neon-purple transition-colors"
                style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              >
                <option value="all" className="bg-bg-deep">ALL DEVICES</option>
                {data.devices?.map((d: any) => (
                  <option key={d.id || d} value={d.id || d} className="bg-bg-deep">{(d.display_name || d.id || d).toUpperCase()}</option>
                ))}
              </select>
            )}
          </div>
        </div>
        <div className="font-mono text-[13px] text-text-dim flex flex-col items-end">
          <div>DATA REFRESH RATE: 5000MS</div>
          <div className="flex gap-[16px] mt-[12px]">
            <button 
              onClick={() => setIsSettingsOpen(true)}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              className="text-white hover:neon-text-cyan cursor-pointer bg-transparent border-none p-0 transition-all"
            >
              [ SETTINGS ]
            </button>
          </div>
        </div>
      </header>
      
      {isSettingsOpen && <SettingsModal onClose={() => setIsSettingsOpen(false)} />}

      {/* PeriodCost Stats Row */}
      <section className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-[24px] mb-[40px]">
        {periods.map((p: any, i: number) => {
          const isElevated = p.label === 'ALL';
          const cardClass = `glass-panel p-[24px] flex flex-col justify-between transition-transform hover:-translate-y-1 ${isElevated ? 'border-neon-purple shadow-[0_0_20px_rgba(176,38,255,0.15)]' : 'border-panel-border hover:border-white/20'}`;
          return (
            <div key={i} className={cardClass}>
              <div className="mb-[16px]">
                <h3 className="text-text-dim text-[14px] tracking-wider uppercase mb-[12px]">{p.label === 'ALL' ? 'TOTAL SPEND' : p.label}</h3>
                <div className="flex flex-col gap-[6px] font-mono text-[13px] text-gray-300">
                  <div className="flex justify-between"><span>IN</span><span className="text-white">{fmt(Math.max(0, p.input_tokens - p.cached_tokens - (p.cache_creation_tokens || 0)))}</span></div>
                  <div className="flex justify-between"><span>CA_R</span><span className="text-neon-cyan">{fmt(p.cached_tokens)}</span></div>
                  <div className="flex justify-between"><span>CA_W</span><span className="text-neon-purple">{fmt(p.cache_creation_tokens || 0)}</span></div>
                  <div className="flex justify-between"><span>OUT</span><span className="text-neon-green">{fmt(p.output_tokens)}</span></div>
                </div>
              </div>
              <div className="font-mono text-[32px] md:text-[36px] font-light text-white leading-none mt-[16px]">
                {fmtCost(p.cost)}
              </div>
            </div>
          )
        })}
      </section>

      {/* LAN Radar Component */}
      <Radar />

      {/* Source Stats Grid */}
      <section className="grid grid-cols-1 xl:grid-cols-2 gap-[32px] mt-[40px]">
        {sources.map((src: any, si: number) => {
           const baseInput = Math.max(0, src.total_input - src.total_cached - (src.total_cache_creation || 0));
           const totalTokens = baseInput + src.total_cached + (src.total_cache_creation || 0) + src.total_output;
           
           const inPct = totalTokens > 0 ? (baseInput / totalTokens) * 100 : 0;
           const cachedPct = totalTokens > 0 ? (src.total_cached / totalTokens) * 100 : 0;
           const cacheCreationPct = totalTokens > 0 ? ((src.total_cache_creation || 0) / totalTokens) * 100 : 0;
           const outPct = totalTokens > 0 ? (src.total_output / totalTokens) * 100 : 0;

           const formatPct = (pct: number) => {
             if (pct > 0 && pct < 1) return '<1%';
             return pct.toFixed(0) + '%';
           };

           const sortedModels = [...(src.models || [])].sort((a: any, b: any) => b.total_cost - a.total_cost);

           return (
            <article key={si} className="glass-panel flex flex-col">
              <div className="p-[24px] border-b border-panel-border flex flex-col md:flex-row justify-between items-start md:items-end gap-[16px] bg-white/5">
                <div>
                  <h2 className="font-display text-[28px] md:text-[36px] font-semibold tracking-tight text-white">
                    {src.name}
                  </h2>
                </div>
                <div className="text-left md:text-right">
                  <span className="text-text-dim text-[12px] tracking-wider uppercase mb-[4px] block">TOTAL SPEND</span>
                  <div className="font-mono text-[28px] md:text-[36px] text-white font-light">{fmtCost(src.total_cost)}</div>
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 border-b border-panel-border">
                {/* Token Distribution Grid */}
                <div className="p-[24px] border-b md:border-b-0 md:border-r border-panel-border grid grid-cols-2 gap-[24px]">
                  <div>
                    <span className="text-text-dim text-[11px] tracking-wider uppercase block mb-[4px]">BASE IN</span>
                    <div className="font-mono text-[20px] text-white">{fmt(baseInput)}</div>
                  </div>
                  <div>
                    <span className="text-text-dim text-[11px] tracking-wider uppercase block mb-[4px]">CA_R</span>
                    <div className="font-mono text-[20px] text-neon-cyan">{fmt(src.total_cached)}</div>
                  </div>
                  <div>
                    <span className="text-text-dim text-[11px] tracking-wider uppercase block mb-[4px]">CA_W</span>
                    <div className="font-mono text-[20px] text-neon-purple">{fmt(src.total_cache_creation || 0)}</div>
                  </div>
                  <div>
                    <span className="text-text-dim text-[11px] tracking-wider uppercase block mb-[4px]">OUTPUT</span>
                    <div className="font-mono text-[20px] text-neon-green">{fmt(src.total_output)}</div>
                  </div>
                </div>

                {/* Smooth Progress Bar Area */}
                <div className="p-[24px] flex flex-col justify-center">
                  <div className="w-full h-[12px] rounded-full bg-black/50 flex overflow-hidden mb-[20px] shadow-inner">
                    <div style={{width: `${inPct}%`}} className="bg-gray-400 h-full"></div>
                    <div style={{width: `${cachedPct}%`}} className="bg-neon-cyan h-full shadow-[0_0_10px_rgba(0,240,255,0.8)]"></div>
                    <div style={{width: `${cacheCreationPct}%`}} className="bg-neon-purple h-full shadow-[0_0_10px_rgba(176,38,255,0.8)]"></div>
                    <div style={{width: `${outPct}%`}} className="bg-neon-green h-full shadow-[0_0_10px_rgba(57,255,20,0.8)]"></div>
                  </div>
                  <div className="grid grid-cols-2 gap-[12px] font-mono text-[12px] text-gray-300">
                    <div className="flex items-center gap-[8px]">
                      <span className="w-[8px] h-[8px] rounded-full bg-gray-400"></span> IN ({formatPct(inPct)})
                    </div>
                    <div className="flex items-center gap-[8px]">
                      <span className="w-[8px] h-[8px] rounded-full bg-neon-cyan"></span> CA_R ({formatPct(cachedPct)})
                    </div>
                    <div className="flex items-center gap-[8px]">
                      <span className="w-[8px] h-[8px] rounded-full bg-neon-purple"></span> CA_W ({formatPct(cacheCreationPct)})
                    </div>
                    <div className="flex items-center gap-[8px]">
                      <span className="w-[8px] h-[8px] rounded-full bg-neon-green"></span> OUT ({formatPct(outPct)})
                    </div>
                  </div>
                </div>
              </div>

              {/* Model Table */}
              <div className="overflow-x-auto p-[16px]">
                <table className="w-full text-left font-mono text-[13px]">
                  <thead>
                    <tr className="text-text-dim border-b border-panel-border/50">
                      <th className="px-[12px] py-[12px] font-normal uppercase tracking-wider">Model Identifier</th>
                      <th className="px-[12px] py-[12px] font-normal uppercase tracking-wider">Rates (1M)</th>
                      <th className="px-[12px] py-[12px] font-normal uppercase tracking-wider text-right">Events</th>
                      <th className="px-[12px] py-[12px] font-normal uppercase tracking-wider text-right">Subtotal</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sortedModels.map((m: any, mi: number) => (
                      <tr key={mi} className="border-b border-panel-border/30 last:border-0 hover:bg-white/5 transition-colors">
                        <td className="px-[12px] py-[12px] text-white">{m.model}</td>
                        <td className="px-[12px] py-[12px] text-gray-400">
                          <span className="text-gray-500">I:</span>{fmtCost(m.input_price_per_m || 0)} 
                          <span className="text-neon-cyan/50 ml-2">CR:</span><span className="text-neon-cyan">{fmtCost(m.cached_price_per_m || 0)}</span> 
                          <span className="text-neon-purple/50 ml-2">CW:</span><span className="text-neon-purple">{fmtCost(m.cache_creation_price_per_m || 0)}</span> 
                          <span className="text-neon-green/50 ml-2">O:</span><span className="text-neon-green">{fmtCost(m.output_price_per_m || 0)}</span>
                        </td>
                        <td className="px-[12px] py-[12px] text-right text-gray-300">{m.events}</td>
                        <td className="px-[12px] py-[12px] text-right text-white font-medium">{fmtCost(m.total_cost)}</td>
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
