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

  if (!data) return (
    <div className="min-h-screen bg-[#FFFFFF] text-[#000000] p-[40px] flex items-center justify-center font-display text-[48px] uppercase border-[24px] border-[#000000] m-[24px]">
      SYSTEM INITIALIZING...
    </div>
  );

  const { periods, sources } = data;

  return (
    <div className="bg-[#FFFFFF] text-[#000000] min-h-screen p-[24px] md:p-[40px] font-sans selection:bg-[#000000] selection:text-[#FFFFFF]">
      
      {/* Header Section */}
      <header className="flex flex-col md:flex-row justify-between items-start md:items-end gap-[24px] mb-[64px] border-b-[5px] border-[#000000] pb-[24px]">
        <div>
          <h1 className="font-display text-[48px] md:text-[64px] leading-[1.0] uppercase tracking-tighter mb-[16px]">
            AI Flight<br />Dashboard
          </h1>
          <div className="flex flex-col md:flex-row md:items-center gap-[16px]">
            <div className="border-[3px] border-[#008000] text-[#008000] px-[12px] py-[4px] font-sans text-[11px] font-semibold uppercase tracking-[1px] w-fit">
              LIVE_OPERATIONS
            </div>
            {data && (
              <select 
                value={selectedDevice} 
                onChange={e => setSelectedDevice(e.target.value)}
                className="bg-[#F0F0F0] text-[#000000] border-[3px] border-[#000000] rounded-none px-[12px] py-[8px] font-mono text-[15px] outline-none focus:border-[5px] focus:m-[-2px]"
              >
                <option value="all">ALL DEVICES</option>
                {data.devices?.map((d: string) => (
                  <option key={d} value={d}>{d.toUpperCase()}</option>
                ))}
              </select>
            )}
          </div>
        </div>
        <div className="font-mono text-[15px] text-left md:text-right">
          <div>DATA REFRESH RATE: 5000MS</div>
          <div className="text-[#0000FF] uppercase underline decoration-[3px] underline-offset-4 cursor-pointer mt-[8px]">
            SYSTEM LOGS
          </div>
        </div>
      </header>

      {/* PeriodCost Stats Row */}
      <section className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-5 gap-[24px] mb-[80px]">
        {periods.map((p: any, i: number) => {
          const isElevated = p.label === 'ALL';
          const cardClass = `bg-[#FFFFFF] border-[#000000] rounded-none p-[24px] flex flex-col justify-between shadow-none ${isElevated ? 'border-[5px]' : 'border-[3px]'}`;
          return (
            <div key={i} className={cardClass}>
              <div className="mb-[16px]">
                <h3 className="font-display text-[22px] leading-[1.1] uppercase mb-[8px]">{p.label}</h3>
                <div className="flex flex-col gap-[4px] font-mono text-[15px]">
                  <span>IN: {fmt(Math.max(0, p.input_tokens - p.cached_tokens - (p.cache_creation_tokens || 0)))}</span>
                  <span>CA_R: {fmt(p.cached_tokens)}</span>
                  <span>CA_W: {fmt(p.cache_creation_tokens || 0)}</span>
                  <span>OUT: {fmt(p.output_tokens)}</span>
                </div>
              </div>
              <div className="font-mono text-[32px] md:text-[40px] leading-none mt-[16px] border-t-[3px] border-[#000000] pt-[16px]">
                {fmtCost(p.cost)}
              </div>
            </div>
          )
        })}
      </section>

      {/* Source Stats Grid */}
      <section className="grid grid-cols-1 xl:grid-cols-2 gap-[40px]">
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

           const sortedModels = [...src.models].sort((a: any, b: any) => b.total_cost - a.total_cost);

           return (
            <article key={si} className="bg-[#FFFFFF] border-[5px] border-[#000000] rounded-none shadow-none flex flex-col">
              <div className="p-[24px] border-b-[5px] border-[#000000] flex flex-col md:flex-row justify-between items-start md:items-end gap-[16px] bg-[#000000] text-[#FFFFFF]">
                <div>
                  <h2 className="font-display text-[32px] md:text-[48px] uppercase leading-[1.05]">
                    {src.name}
                  </h2>
                </div>
                <div className="text-left md:text-right">
                  <span className="font-display text-[14px] uppercase mb-[4px] block">TOTAL SPEND</span>
                  <div className="font-mono text-[32px] md:text-[48px] leading-none">{fmtCost(src.total_cost)}</div>
                </div>
              </div>

              <div className="grid grid-cols-1 md:grid-cols-2 border-b-[5px] border-[#000000]">
                {/* Token Distribution Grid */}
                <div className="p-[24px] border-b-[5px] md:border-b-0 md:border-r-[5px] border-[#000000] grid grid-cols-2 md:grid-cols-3 gap-[24px]">
                  <div className="border-l-[5px] border-[#000000] pl-[12px]">
                    <span className="font-display text-[14px] uppercase block mb-[4px]">BASE IN</span>
                    <div className="font-mono text-[24px]">{fmt(baseInput)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-[12px]">
                    <span className="font-display text-[14px] uppercase block mb-[4px]">CA_R</span>
                    <div className="font-mono text-[24px]">{fmt(src.total_cached)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-[12px]">
                    <span className="font-display text-[14px] uppercase block mb-[4px]">CA_W</span>
                    <div className="font-mono text-[24px]">{fmt(src.total_cache_creation || 0)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-[12px]">
                    <span className="font-display text-[14px] uppercase block mb-[4px]">OUTPUT</span>
                    <div className="font-mono text-[24px]">{fmt(src.total_output)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-[12px]">
                    <span className="font-display text-[14px] uppercase block mb-[4px]">TOTAL</span>
                    <div className="font-mono text-[24px]">{fmt(totalTokens)}</div>
                  </div>
                </div>

                {/* Brutalist Progress Bar Area */}
                <div className="p-[24px] flex flex-col justify-center">
                  <div className="w-full h-[40px] border-[3px] border-[#000000] flex mb-[16px]">
                    <div style={{width: `${inPct}%`}} className="bg-[#000000] h-full border-r-[3px] border-[#000000]"></div>
                    <div style={{width: `${cachedPct}%`}} className="bg-[#CCCCCC] h-full border-r-[3px] border-[#000000]"></div>
                    <div style={{width: `${cacheCreationPct}%`}} className="bg-[#888888] h-full border-r-[3px] border-[#000000]"></div>
                    <div style={{width: `${outPct}%`}} className="bg-[#FFFFFF] h-full"></div>
                  </div>
                  <div className="flex flex-col gap-[8px] font-mono text-[15px]">
                    <div className="flex items-center gap-[8px]">
                      <span className="w-[16px] h-[16px] bg-[#000000] border-[3px] border-[#000000]"></span> IN ({formatPct(inPct)})
                    </div>
                    <div className="flex items-center gap-[8px]">
                      <span className="w-[16px] h-[16px] bg-[#CCCCCC] border-[3px] border-[#000000]"></span> CA_R ({formatPct(cachedPct)})
                    </div>
                    <div className="flex items-center gap-[8px]">
                      <span className="w-[16px] h-[16px] bg-[#888888] border-[3px] border-[#000000]"></span> CA_W ({formatPct(cacheCreationPct)})
                    </div>
                    <div className="flex items-center gap-[8px]">
                      <span className="w-[16px] h-[16px] bg-[#FFFFFF] border-[3px] border-[#000000]"></span> OUT ({formatPct(outPct)})
                    </div>
                  </div>
                </div>
              </div>

              {/* Model Table */}
              <div className="overflow-x-auto">
                <table className="w-full text-left font-mono text-[15px]">
                  <thead>
                    <tr className="border-b-[5px] border-[#000000] bg-[#F0F0F0]">
                      <th className="px-[16px] py-[16px] font-display text-[14px] uppercase">Model Identifier</th>
                      <th className="px-[16px] py-[16px] font-display text-[14px] uppercase">Rates (1M)</th>
                      <th className="px-[16px] py-[16px] font-display text-[14px] uppercase">Events</th>
                      <th className="px-[16px] py-[16px] font-display text-[14px] uppercase text-right">Subtotal</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sortedModels.map((m: any, mi: number) => (
                      <tr key={mi} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#000000] hover:text-[#FFFFFF] transition-none group">
                        <td className="px-[16px] py-[16px] font-bold group-hover:text-[#FFFFFF]">{m.model}</td>
                        <td className="px-[16px] py-[16px] group-hover:text-[#FFFFFF]">
                          IN: {fmtCost(m.input_price_per_m || 0)} / CA_R: {fmtCost(m.cached_price_per_m || 0)} / CA_W: {fmtCost(m.cache_creation_price_per_m || 0)} / OUT: {fmtCost(m.output_price_per_m || 0)}
                        </td>
                        <td className="px-[16px] py-[16px] group-hover:text-[#FFFFFF]">{m.events}</td>
                        <td className="px-[16px] py-[16px] text-right font-bold group-hover:text-[#FFFFFF]">{fmtCost(m.total_cost)}</td>
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

