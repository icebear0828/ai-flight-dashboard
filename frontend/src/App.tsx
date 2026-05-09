import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import SettingsModal from "./SettingsModal";
import Radar from "./components/Radar";

const num = (value: unknown): number => {
  const n = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(n) ? n : 0;
};

const text = (value: unknown, fallback = ''): string => {
  return typeof value === 'string' && value.trim() !== '' ? value : fallback;
};

const fmt = (value: unknown) => {
  const n = num(value);
  if (n >= 1e9) return (n/1e9).toFixed(2) + 'B';
  if (n >= 1e6) return (n/1e6).toFixed(2) + 'M';
  if (n >= 1e3) return (n/1e3).toFixed(1) + 'K';
  return n.toString();
};
const fmtCost = (value: unknown) => '$' + num(value).toFixed(2);
const fmtPercent = (value: unknown) => num(value).toFixed(1) + '%';

type JsonRecord = Record<string, unknown>;

const asRecord = (value: unknown): JsonRecord => {
  return value !== null && typeof value === 'object' ? value as JsonRecord : {};
};

type WailsWindow = Window & {
  go?: {
    desktop?: {
      App?: {
        OpenSystemLogs?: () => void;
      };
    };
  };
};

const wailsWindow = (): WailsWindow => window as WailsWindow;

interface PeriodStats {
  label: string;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens?: number;
  output_tokens: number;
  cost: number;
  cache_hit_rate: number;
}

interface SourceModelStats {
  model: string;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens?: number;
  output_tokens: number;
  input_price_per_m?: number;
  cached_price_per_m?: number;
  cache_creation_price_per_m?: number;
  output_price_per_m?: number;
  events: number;
  total_cost: number;
  cache_hit_rate: number;
}

interface SourceStats {
  name: string;
  total_input: number;
  total_cached: number;
  total_cache_creation?: number;
  total_output: number;
  total_cost: number;
  total_events: number;
  cache_hit_rate: number;
  models?: SourceModelStats[];
}

interface DeviceStats {
  id: string;
  display_name?: string;
}

interface ProjectStat {
  project: string;
  events: number;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens?: number;
  output_tokens: number;
  total_cost: number;
  cache_hit_rate: number;
}

interface DashboardData {
  periods: PeriodStats[];
  sources: SourceStats[];
  devices: DeviceStats[];
  projects?: ProjectStat[];
  is_paused?: boolean;
}

const normalizeDashboardData = (raw: unknown): DashboardData => {
  const root = asRecord(raw);
  const periods = Array.isArray(root.periods) ? root.periods : [];
  const sources = Array.isArray(root.sources) ? root.sources : [];
  const devices = Array.isArray(root.devices) ? root.devices : [];
  const projects = Array.isArray(root.projects) ? root.projects : [];

  return {
    periods: periods.map((period) => {
      const p = asRecord(period);
      return {
        label: text(p.label, 'UNKNOWN'),
        input_tokens: num(p.input_tokens),
        cached_tokens: num(p.cached_tokens),
        cache_creation_tokens: num(p.cache_creation_tokens),
        output_tokens: num(p.output_tokens),
        cost: num(p.cost),
        cache_hit_rate: num(p.cache_hit_rate),
      };
    }),
    sources: sources.map((source) => {
      const src = asRecord(source);
      const models = Array.isArray(src.models) ? src.models : [];
      return {
        name: text(src.name, 'Unknown'),
        total_input: num(src.total_input),
        total_cached: num(src.total_cached),
        total_cache_creation: num(src.total_cache_creation),
        total_output: num(src.total_output),
        total_cost: num(src.total_cost),
        total_events: num(src.total_events),
        cache_hit_rate: num(src.cache_hit_rate),
        models: models.map((model) => {
          const m = asRecord(model);
          return {
            model: text(m.model, 'unknown'),
            input_tokens: num(m.input_tokens),
            cached_tokens: num(m.cached_tokens),
            cache_creation_tokens: num(m.cache_creation_tokens),
            output_tokens: num(m.output_tokens),
            input_price_per_m: num(m.input_price_per_m),
            cached_price_per_m: num(m.cached_price_per_m),
            cache_creation_price_per_m: num(m.cache_creation_price_per_m),
            output_price_per_m: num(m.output_price_per_m),
            events: num(m.events),
            total_cost: num(m.total_cost),
            cache_hit_rate: num(m.cache_hit_rate),
          };
        }),
      };
    }),
    devices: devices.map((device) => {
      const d = asRecord(device);
      const fallbackID = text(device, 'local');
      const id = text(d.id, fallbackID);
      return {
        id,
        display_name: text(d.display_name, id),
      };
    }),
    projects: projects.map((project) => {
      const p = asRecord(project);
      return {
        project: text(p.project, 'Default'),
        events: num(p.events),
        input_tokens: num(p.input_tokens),
        cached_tokens: num(p.cached_tokens),
        cache_creation_tokens: num(p.cache_creation_tokens),
        output_tokens: num(p.output_tokens),
        total_cost: num(p.total_cost),
        cache_hit_rate: num(p.cache_hit_rate),
      };
    }),
    is_paused: Boolean(root.is_paused),
  };
};

export default function App() {
  const { t, i18n } = useTranslation();
  const [data, setData] = useState<DashboardData | null>(null);
  const [selectedDevice, setSelectedDevice] = useState<string>("all");
  const [selectedSource, setSelectedSource] = useState<string>("");
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [errorMsg, setErrorMsg] = useState<string>("");
  
  useEffect(() => {
    const fetchData = async () => {
      try {
        // Works in both Wails (assets handler) and standalone Web mode
        const params = new URLSearchParams({ device: selectedDevice });
        if (selectedSource) params.set('source', selectedSource);
        const res = await fetch("/api/stats?" + params.toString());
        if (!res.ok) {
           throw new Error(`HTTP ${res.status}: ${await res.text()}`);
        }
        const json = await res.json();
        setData(normalizeDashboardData(json));
        setErrorMsg("");
      } catch (e: unknown) {
        console.error(e);
        setErrorMsg(e instanceof Error ? e.toString() : String(e));
      }
    };
    fetchData();
    const interval = setInterval(fetchData, 5000);
    return () => clearInterval(interval);
  }, [selectedDevice, selectedSource]);

  const togglePause = async () => {
		try {
			const res = await fetch("/api/pause", { method: "POST" });
			if (res.ok) {
				const json = await res.json();
				setData(prev => prev ? { ...prev, is_paused: json.is_paused } : null);
			}
		} catch (e) {
			console.error("Failed to toggle pause", e);
		}
	};

  // Detect if we are running inside the Wails desktop app
  const isDesktop = typeof window !== 'undefined' && wailsWindow().go !== undefined;

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
          <h1 className="font-display text-5xl sm:text-6xl md:text-7xl leading-[1.0] uppercase tracking-tighter mb-4 break-words">
            {t('aiFlightDashboard').split(' ').map((word, i) => (
              <React.Fragment key={i}>{i > 0 && <br/>}{word}</React.Fragment>
            ))}
          </h1>
          <div className="flex flex-col sm:flex-row sm:items-center gap-4">
            <button 
              onClick={togglePause}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              className={`border-[3px] px-3 py-1 font-sans text-xs font-semibold uppercase tracking-wider w-fit min-w-[100px] text-center cursor-pointer ${data?.is_paused ? 'border-[#FF0000] text-[#FF0000] hover:bg-[#FF0000] hover:text-[#FFFFFF]' : 'border-[#008000] text-[#008000] hover:bg-[#008000] hover:text-[#FFFFFF]'}`}
            >
              {data?.is_paused ? t('paused') : t('liveOperations')}
            </button>
            {data && (
              <select 
                value={selectedDevice} 
                onChange={e => setSelectedDevice(e.target.value)}
                className="bg-[#F0F0F0] text-[#000000] border-[3px] border-[#000000] rounded-none px-3 py-2 font-mono text-sm md:text-base outline-none focus:border-[5px] focus:m-[-2px] min-w-[140px]"
                style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              >
                <option value="all">{t('allDevices')}</option>
                {data.devices?.map((d: DeviceStats) => (
                  <option key={d.id} value={d.id}>{(d.display_name || d.id).toUpperCase()}</option>
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
              className="text-[#0000FF] uppercase underline decoration-[3px] underline-offset-4 cursor-pointer bg-transparent border-none p-0 hover:text-[#000000] whitespace-nowrap"
            >
              [ {t('settings')} ]
            </button>
            <button 
              onClick={() => i18n.changeLanguage(i18n.language === 'zh' ? 'en' : 'zh')}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
              className="text-[#0000FF] uppercase underline decoration-[3px] underline-offset-4 cursor-pointer bg-transparent border-none p-0 hover:text-[#000000] whitespace-nowrap"
            >
              [ {i18n.language === 'zh' ? 'EN' : '中'} ]
            </button>
            <div 
              onClick={() => {
                const app = wailsWindow().go?.desktop?.App;
                if (app?.OpenSystemLogs) {
                  app.OpenSystemLogs();
                } else {
                  console.log("OpenSystemLogs not available in web mode");
                }
              }}
              className="text-[#0000FF] uppercase underline decoration-[3px] underline-offset-4 cursor-pointer hover:text-[#000000] whitespace-nowrap"
            >
              [ {t('systemLogs')} ]
            </div>
          </div>
        </div>
      </header>
      
      {isSettingsOpen && <SettingsModal onClose={() => setIsSettingsOpen(false)} />}

      {/* Source Filter Tabs + PeriodCost Stats */}
      <section className="mb-12 md:mb-20">
        <div className="flex items-center gap-0 mb-6">
          {[
            { label: t('total'), value: '' },
            { label: 'CLAUDE', value: 'Claude Code' },
            { label: 'GEMINI', value: 'Gemini CLI' },
            { label: 'CODEX', value: 'Codex' },
          ].map((tab) => (
            <button
              key={tab.value}
              onClick={() => setSelectedSource(tab.value)}
              className={`px-4 py-2 font-display text-sm uppercase tracking-wider border-[3px] border-[#000000] cursor-pointer transition-none -ml-[3px] first:ml-0 ${
                selectedSource === tab.value
                  ? 'bg-[#000000] text-[#FFFFFF]'
                  : 'bg-[#FFFFFF] text-[#000000] hover:bg-[#F0F0F0]'
              }`}
              style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 xl:grid-cols-5 gap-4 md:gap-6">
        {periods.map((p: PeriodStats, i: number) => {
          const isElevated = p.label === 'ALL';
          const cardClass = `bg-[#FFFFFF] border-[#000000] rounded-none p-4 md:p-6 flex flex-col justify-between shadow-none ${isElevated ? 'border-[5px]' : 'border-[3px]'}`;
          return (
            <div key={i} className={cardClass}>
              <div className="mb-4">
                <h3 className="font-display text-xl xl:text-2xl leading-[1.1] uppercase mb-2">{p.label}</h3>
                <div className="flex flex-col gap-1 font-mono text-xs xl:text-sm">
                  <span>{t('labelIn')}: {fmt(Math.max(0, num(p.input_tokens) - num(p.cached_tokens) - num(p.cache_creation_tokens)))}</span>
                  <span>{t('cacheRead')}: {fmt(p.cached_tokens)}</span>
                  <span>{t('cacheWrite')}: {fmt(p.cache_creation_tokens)}</span>
                  <span>{t('labelOut')}: {fmt(p.output_tokens)}</span>
                  <span>{t('cacheHitRate')}: {fmtPercent(p.cache_hit_rate)}</span>
                </div>
              </div>
              <div className="font-mono text-2xl md:text-3xl leading-none mt-4 border-t-[3px] border-[#000000] pt-4 whitespace-nowrap overflow-hidden text-ellipsis">
                {fmtCost(p.cost)}
              </div>
            </div>
          )
        })}
        </div>
      </section>

      {/* LAN Radar Component */}
      <Radar />

      {/* Projects Section */}
      {data.projects && data.projects.length > 0 && (
        <section className="mb-12 md:mb-20">
          <article className="bg-[#FFFFFF] border-[5px] border-[#000000] rounded-none shadow-none flex flex-col">
            <div className="p-4 sm:p-6 border-b-[5px] border-[#000000] flex flex-col md:flex-row justify-between items-start md:items-end gap-4 bg-[#000000] text-[#FFFFFF]">
              <div className="break-words w-full">
                <h2 className="font-display text-3xl sm:text-4xl md:text-5xl uppercase leading-[1.05] break-words">
                  {t('projects') || 'PROJECT STATS'}
                </h2>
              </div>
            </div>
            <div className="overflow-x-auto">
              <table className="w-full text-left font-mono text-xs sm:text-sm min-w-[600px]">
                <thead>
                  <tr className="border-b-[5px] border-[#000000] bg-[#F0F0F0]">
                    <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('project')}</th>
                    <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('events')}</th>
                    <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('tokens')}</th>
                    <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase text-right">{t('subtotal')}</th>
                  </tr>
                </thead>
                <tbody>
                  {data.projects.map((p) => (
                    <tr key={p.project} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#000000] hover:text-[#FFFFFF] transition-none group">
                      <td className="px-3 py-3 sm:px-4 sm:py-4 font-bold group-hover:text-[#FFFFFF] truncate max-w-[300px]" title={p.project}>{p.project}</td>
                      <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">{p.events}</td>
                      <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">
                        {t('labelIn')}: {fmt(Math.max(0, num(p.input_tokens) - num(p.cached_tokens) - num(p.cache_creation_tokens)))} / {t('cacheRead')}: {fmt(p.cached_tokens)} / {t('cacheWrite')}: {fmt(p.cache_creation_tokens)} / {t('labelOut')}: {fmt(p.output_tokens)} / {t('cacheHitRate')}: {fmtPercent(p.cache_hit_rate)}
                      </td>
                      <td className="px-3 py-3 sm:px-4 sm:py-4 text-right font-bold group-hover:text-[#FFFFFF]">{fmtCost(p.total_cost)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </article>
        </section>
      )}

      {/* Source Stats Grid */}
      <section className="grid grid-cols-1 2xl:grid-cols-2 gap-6 lg:gap-10">
        {sources.map((src: SourceStats, si: number) => {
           const baseInput = Math.max(0, num(src.total_input) - num(src.total_cached) - num(src.total_cache_creation));
           const totalTokens = baseInput + num(src.total_cached) + num(src.total_cache_creation) + num(src.total_output);
           
           const inPct = totalTokens > 0 ? (baseInput / totalTokens) * 100 : 0;
           const cachedPct = totalTokens > 0 ? (num(src.total_cached) / totalTokens) * 100 : 0;
           const cacheCreationPct = totalTokens > 0 ? (num(src.total_cache_creation) / totalTokens) * 100 : 0;
           const outPct = totalTokens > 0 ? (num(src.total_output) / totalTokens) * 100 : 0;

           const formatPct = (pct: number) => {
             if (pct > 0 && pct < 1) return '<1%';
             return pct.toFixed(0) + '%';
           };

           const sortedModels = [...(src.models || [])].sort((a: SourceModelStats, b: SourceModelStats) => num(b.total_cost) - num(a.total_cost));

           return (
            <article key={si} className="bg-[#FFFFFF] border-[5px] border-[#000000] rounded-none shadow-none flex flex-col">
              <div className="p-4 sm:p-6 border-b-[5px] border-[#000000] flex flex-col md:flex-row justify-between items-start md:items-end gap-4 bg-[#000000] text-[#FFFFFF]">
                <div className="break-words w-full">
                  <h2 className="font-display text-3xl sm:text-4xl md:text-5xl uppercase leading-[1.05] break-words">
                    {src.name}
                  </h2>
                </div>
                <div className="text-left md:text-right shrink-0">
                  <span className="font-display text-xs sm:text-sm uppercase mb-1 block">{t('totalSpend')}</span>
                  <div className="font-mono text-3xl sm:text-4xl md:text-5xl leading-none">{fmtCost(src.total_cost)}</div>
                </div>
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-2 border-b-[5px] border-[#000000]">
                {/* Token Distribution Grid */}
                <div className="p-4 sm:p-6 border-b-[5px] lg:border-b-0 lg:border-r-[5px] border-[#000000] grid grid-cols-2 sm:grid-cols-3 gap-4 sm:gap-6">
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('baseInput')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(baseInput)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('cacheRead')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(src.total_cached)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('cacheWrite')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(src.total_cache_creation)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('outputTokens')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(src.total_output)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('totalTokens')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmt(totalTokens)}</div>
                  </div>
                  <div className="border-l-[5px] border-[#000000] pl-3">
                    <span className="font-display text-xs sm:text-sm uppercase block mb-1">{t('cacheHitRate')}</span>
                    <div className="font-mono text-xl sm:text-2xl">{fmtPercent(src.cache_hit_rate)}</div>
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
                      <span className="w-4 h-4 bg-[#000000] border-[3px] border-[#000000] shrink-0"></span> {t('labelIn')} ({formatPct(inPct)})
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#CCCCCC] border-[3px] border-[#000000] shrink-0"></span> {t('cacheRead')} ({formatPct(cachedPct)})
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#888888] border-[3px] border-[#000000] shrink-0"></span> {t('cacheWrite')} ({formatPct(cacheCreationPct)})
                    </div>
                    <div className="flex items-center gap-2">
                      <span className="w-4 h-4 bg-[#FFFFFF] border-[3px] border-[#000000] shrink-0"></span> {t('labelOut')} ({formatPct(outPct)})
                    </div>
                  </div>
                </div>
              </div>

              {/* Model Table */}
              <div className="overflow-x-auto">
                <table className="w-full text-left font-mono text-xs sm:text-sm min-w-[760px]">
                  <thead>
                    <tr className="border-b-[5px] border-[#000000] bg-[#F0F0F0]">
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('modelIdentifier')}</th>
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('rates1M')}</th>
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('tokens')}</th>
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase">{t('events')}</th>
                      <th className="px-3 py-3 sm:px-4 sm:py-4 font-display text-xs sm:text-sm uppercase text-right">{t('subtotal')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {sortedModels.map((m: SourceModelStats, mi: number) => (
                      <tr key={mi} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#000000] hover:text-[#FFFFFF] transition-none group">
                        <td className="px-3 py-3 sm:px-4 sm:py-4 font-bold group-hover:text-[#FFFFFF] max-w-[200px] truncate" title={m.model}>{m.model}</td>
                        <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">
                          {t('labelIn')}: {fmtCost(m.input_price_per_m)} / {t('cacheRead')}: {fmtCost(m.cached_price_per_m)} / {t('cacheWrite')}: {fmtCost(m.cache_creation_price_per_m)} / {t('labelOut')}: {fmtCost(m.output_price_per_m)}
                        </td>
                        <td className="px-3 py-3 sm:px-4 sm:py-4 group-hover:text-[#FFFFFF]">
                          {t('labelIn')}: {fmt(Math.max(0, num(m.input_tokens) - num(m.cached_tokens) - num(m.cache_creation_tokens)))} / {t('cacheRead')}: {fmt(m.cached_tokens)} / {t('cacheWrite')}: {fmt(m.cache_creation_tokens)} / {t('labelOut')}: {fmt(m.output_tokens)} / {t('cacheHitRate')}: {fmtPercent(m.cache_hit_rate)}
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
