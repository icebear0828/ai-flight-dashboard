import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
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
  const { t, i18n } = useTranslation();
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

  // Detect if we are running inside the Wails desktop app
  const isDesktop = typeof window !== 'undefined' && (window as any).go !== undefined;

  if (errorMsg) return (
    <div className={`min-h-screen bg-[#FFFFFF] text-[#FF0000] p-4 sm:p-6 md:p-10 flex items-center justify-center font-display text-xl sm:text-2xl uppercase border-[12px] border-[#FF0000] m-3 ${isDesktop ? 'wails-drag' : ''}`}>
      <div className="text-center">
        <div className="text-4xl md:text-5xl mb-4 text-[#000000] bg-[#FF0000] inline-block px-6 py-2">{t('systemError')}</div>
        <br/> {errorMsg}
      </div>
    </div>
  );

  if (!data) return (
    <div className={`min-h-screen bg-[#FFFFFF] text-[#000000] p-4 sm:p-6 md:p-10 flex items-center justify-center font-display text-4xl sm:text-5xl md:text-6xl uppercase border-[12px] border-[#000000] m-3 ${isDesktop ? 'wails-drag' : ''}`}>
      {t('systemInitializing')}
    </div>
  );

  const { periods = [], sources = [] } = data;

  return (
    <div className={`bg-[#FFFFFF] text-[#000000] min-h-screen px-4 pb-4 sm:px-6 sm:pb-6 md:px-10 md:pb-10 font-sans selection:bg-[#000000] selection:text-[#FFFFFF] ${isDesktop ? 'pt-12' : 'pt-6 md:pt-10'}`}>
      
      {/* Dedicated invisible draggable titlebar for macOS native window controls */}
      {isDesktop && (
        <div className="h-10 w-full wails-drag fixed top-0 left-0 z-50 bg-[#FFFFFF]"></div>
      )}

      {/* Header Section */}
      <header className="flex flex-col md:flex-row justify-between items-start md:items-end gap-6 mb-16 border-b-[5px] border-[#000000] pb-6">
        <div>
          <h1 className="font-display text-5xl sm:text-6xl md:text-7xl leading-[1.0] uppercase tracking-tighter mb-4 break-words" dangerouslySetInnerHTML={{ __html: t('aiFlightDashboard').replace(' ', '<br/>') }}>
          </h1>
          <div className="flex flex-col sm:flex-row sm:items-center gap-4">
            <div className="border-[3px] border-[#008000] text-[#008000] px-3 py-1 font-sans text-xs font-semibold uppercase tracking-wider w-fit min-w-[100px] text-center">
              {t('liveOperations')}
            </div>
            {data && (
              <select 
                value={selectedDevice} 
                onChange={e => setSelectedDevice(e.target.value)}
                className="bg-[#F0F0F0] text-[#000000] border-[3px] border-[#000000] rounded-none px-3 py-2 font-mono text-sm md:text-base outline-none focus:border-[5px] focus:m-[-2px] min-w-[140px]"
                style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              >
                <option value="all">{t('allDevices')}</option>
                {data.devices?.map((d: any) => (
                  <option key={d.id || d} value={d.id || d}>{(d.display_name || d.id || d).toUpperCase()}</option>
                ))}
              </select>
            )}
          </div>
        </div>
        <div className="font-mono text-sm md:text-base text-left md:text-right flex flex-col items-start md:items-end w-full md:w-auto mt-4 md:mt-0">
          <div>{t('dataRefreshRate')}</div>
          <div className="flex flex-wrap gap-4 mt-2">
            <button 
              onClick={() => setIsSettingsOpen(true)}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              className="text-[#0000FF] uppercase underline decoration-[3px] underline-offset-4 cursor-pointer bg-transparent border-none p-0 hover:text-[#000000]"
            >
              [ {t('settings')} ]
            </button>
            <button 
              onClick={() => i18n.changeLanguage(i18n.language === 'zh' ? 'en' : 'zh')}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              className="text-[#0000FF] uppercase underline decoration-[3px] underline-offset-4 cursor-pointer bg-transparent border-none p-0 hover:text-[#000000] w-12 text-center"
            >
              [ {i18n.language === 'zh' ? 'EN' : '中'} ]
            </button>
            <div className="text-[#0000FF] uppercase underline decoration-[3px] underline-offset-4 cursor-pointer hover:text-[#000000]">
              [ {t('systemLogs')} ]
            </div>
          </div>
        </div>
      </header>
      
      {isSettingsOpen && <SettingsModal onClose={() => setIsSettingsOpen(false)} />}

      {/* PeriodCost Stats Row */}
      <section className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-5 gap-4 md:gap-6 mb-12 md:mb-20">
        {periods.map((p: any, i: number) => {
          const isElevated = p.label === 'ALL';
          const cardClass = `bg-[#FFFFFF] border-[#000000] rounded-none p-4 md:p-6 flex flex-col justify-between shadow-none ${isElevated ? 'border-[5px]' : 'border-[3px]'}`;
          return (
            <div key={i} className={cardClass}>
              <div className="mb-4">
                <h3 className="font-display text-xl xl:text-2xl leading-[1.1] uppercase mb-2">{p.label}</h3>
                <div className="flex flex-col gap-1 font-mono text-xs xl:text-sm">
                  <span>IN: {fmt(Math.max(0, p.input_tokens - p.cached_tokens - (p.cache_creation_tokens || 0)))}</span>
                  <span>CA_R: {fmt(p.cached_tokens)}</span>
                  <span>CA_W: {fmt(p.cache_creation_tokens || 0)}</span>
                  <span>OUT: {fmt(p.output_tokens)}</span>
                </div>
              </div>
              <div className="font-mono text-3xl xl:text-4xl leading-none mt-4 border-t-[3px] border-[#000000] pt-4 break-words">
                {fmtCost(p.cost)}
              </div>
            </div>
          )
        })}
      </section>

      {/* LAN Radar Component */}
      <Radar />

      {/* Source Stats Grid */}
      <section className="grid grid-cols-1 2xl:grid-cols-2 gap-6 lg:gap-10">
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
            <article key={si} className="bg-[#FFFFFF] border-[5px] border-[#000000] rounded-none shadow-none flex flex-col">
              <div className="p-4 sm:p-6 border-b-[5px] border-[#000000] flex flex-col md:flex-row justify-between items-start md:items-end gap-4 bg-[#000000] text-[#FFFFFF]">
                <div className="break-words w-full">
                  <h2 className="font-display text-3xl sm:text-4xl md:text-5xl uppercase leading-[1.05] break-words">
                    {src.name}
                  </h2>
                </div>
                <div className="text-left md:text-right shrink-0">
                  <span className="font-display text-xs sm:text-sm uppercase mb-1 block">TOTAL SPEND</span>
                  <div className="font-mono text-3xl sm:text-4xl md:text-5xl leading-none">{fmtCost(src.total_cost)}</div>
                </div>
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-2 border-b-[5px] border-[#000000]">
                {/* Token Distribution Grid */}
                <div className="p-4 sm:p-6 border-b-[5px] lg:border-b-0 lg:border-r-[5px] border-[#000000] grid grid-cols-2 sm:grid-cols-3 gap-4 sm:gap-6">
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">BASE IN</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(baseInput)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">CA_R</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(src.total_cached)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">CA_W</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(src.total_cache_creation || 0)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">OUTPUT</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(src.total_output)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">TOTAL</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(totalTokens)}</div>
                  </div>
                </div>

                {/* Brutalist Progress Bar Area */}
                <div className="p-4 sm:p-6 flex flex-col justify-center">
                  <div className="w-full h-8 sm:h-10 border-[3px] border-[#000000] flex mb-4">
                    <div style={{width: `${inPct}%`}} className="bg-[#000000] h-full border-r-[3px] border-[#000000]"></div>
                    <div style={{width: `${cachedPct}%`}} className="bg-[#CCCCCC] h-full border-r-[3px] border-[#000000]"></div>
                    <div style={{width: `${cacheCreationPct}%`}} className="bg-[#888888] h-full border-r-[3px] border-[#000000]"></div>
                    <div style={{width: `${outPct}%`}} className="bg-[#FFFFFF] h-full"></div>
                  </div>
                  <div className="flex flex-col gap-2 font-mono text-xs sm:text-sm">
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#000000] border-[3px] border-[#000000] shrink-0"></span> IN ({formatPct(inPct)})
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#CCCCCC] border-[3px] border-[#000000] shrink-0"></span> CA_R ({formatPct(cachedPct)})
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#888888] border-[3px] border-[#000000] shrink-0"></span> CA_W ({formatPct(cacheCreationPct)})
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#FFFFFF] border-[3px] border-[#000000] shrink-0"></span> OUT ({formatPct(outPct)})
                    </div>
                  </div>
                </div>
              </div>

              {/* Model Table */}
              <div className="overflow-x-auto">
                <table className="w-full text-left font-mono text-xs sm:text-sm min-w-[600px]">
                  <thead>
                    <tr className="border-b-[5px] border-[#000000] bg-[#F0F0F0]">
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">Model Identifier</th>
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">Rates (1M)</th>
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">Events</th>
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase text-right">Subtotal</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sortedModels.map((m: any, mi: number) => (
                      <tr key={mi} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#000000] hover:text-[#FFFFFF] transition-none group">
                        <td className="px-3 py-3 sm:px-4 sm:py-4 font-bold group-hover:text-[#FFFFFF] max-w-[200px] truncate" title={m.model}>{m.model}</td>
                        <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">
                          IN: {fmtCost(m.input_price_per_m || 0)} / CA_R: {fmtCost(m.cached_price_per_m || 0)} / CA_W: {fmtCost(m.cache_creation_price_per_m || 0)} / OUT: {fmtCost(m.output_price_per_m || 0)}
                        </td>
                        <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">{m.events}</td>
                        <td className="px-3 py-3 sm:px-4 sm:py-4 text-right font-bold group-hover:text-[#FFFFFF]">{fmtCost(m.total_cost)}</td>
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
