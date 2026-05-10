import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

interface LanPeer {
  id: string;
  display_name?: string;
  ip?: string;
  http_port?: number;
  sync_status?: string;
  sync_error?: string;
  tokens_24h?: number;
  tokens_total?: number;
  cost_total?: number;
  sources?: LanSourceSummary[];
}

interface LanSourceSummary {
  source: string;
  tokens_24h?: number;
  tokens_total?: number;
  cost_total?: number;
}

const num = (value: unknown): number => {
  const n = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(n) ? n : 0;
};

const record = (value: unknown): Record<string, unknown> => {
  return value !== null && typeof value === 'object' ? value as Record<string, unknown> : {};
};

const fmt = (value: unknown) => {
  const n = num(value);
  if (n >= 1e9) return (n / 1e9).toFixed(2) + 'B';
  if (n >= 1e6) return (n / 1e6).toFixed(2) + 'M';
  if (n >= 1e3) return (n / 1e3).toFixed(1) + 'K';
  return n.toString();
};

const statusClass = (status?: string) => {
  if (status === 'ok') return 'text-[#008000]';
  if (status === 'syncing' || status === 'pending' || status === 'discovery_only') return 'text-[#0000FF]';
  return 'text-[#FF0000]';
};

export default function Radar() {
  const { t } = useTranslation();
  const [peers, setPeers] = useState<LanPeer[]>([]);
  const [joined, setJoined] = useState(false);
  const [updatingNetwork, setUpdatingNetwork] = useState(false);

  useEffect(() => {
    let cancelled = false;

    const fetchStatus = async () => {
      try {
        const res = await fetch('/api/lan/status');
        if (!res.ok) return;
        const data = record(await res.json());
        if (typeof data.enabled === 'boolean' && !cancelled) {
          setJoined(data.enabled);
          if (!data.enabled) {
            setPeers([]);
          }
        }
      } catch (e) {
        console.error(e);
      }
    };

    const fetchPeers = async () => {
      try {
        const res = await fetch('/api/lan/scan');
        const data = await res.json();
        if (cancelled) return;
        if (Array.isArray(data.peer_infos)) {
          setPeers(data.peer_infos);
        } else if (Array.isArray(data.peers)) {
          setPeers(data.peers.map((id: string) => ({ id })));
        }
      } catch (e) {
        console.error(e);
      }
    };
    
    fetchStatus();
    fetchPeers();
    const interval = setInterval(() => {
      fetchStatus();
      fetchPeers();
    }, 3000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, []);

  const applyStatus = (raw: unknown, fallback: boolean) => {
    const data = record(raw);
    setJoined(typeof data.enabled === 'boolean' ? data.enabled : fallback);
  };

  const handleJoin = async () => {
    setUpdatingNetwork(true);
    try {
      const res = await fetch('/api/lan/join', { method: 'POST' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      applyStatus(await res.json(), true);
    } catch (e) {
      console.error(e);
    } finally {
      setUpdatingNetwork(false);
    }
  };

  const handleLeave = async () => {
    setUpdatingNetwork(true);
    try {
      const res = await fetch('/api/lan/leave', { method: 'POST' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      applyStatus(await res.json(), false);
      setPeers([]);
    } catch (e) {
      console.error(e);
    } finally {
      setUpdatingNetwork(false);
    }
  };

  return (
    <div className="border-[5px] border-[#000000] p-4 sm:p-6 md:p-10 bg-[#FFFFFF] flex flex-col items-center mb-16 md:mb-20">
      <div className="w-full flex justify-between items-center border-b-[3px] border-[#000000] pb-4 mb-8">
        <h2 className="font-display text-2xl sm:text-3xl uppercase">{t('lanRadar')}</h2>
        {!joined ? (
          <button 
            onClick={handleJoin}
            disabled={updatingNetwork}
            className="border-[3px] border-[#000000] bg-[#000000] text-[#FFFFFF] px-4 py-2 sm:px-6 sm:py-2 font-bold text-sm sm:text-base uppercase hover:bg-[#333333] transition-none cursor-pointer disabled:opacity-50"
          >
            {t('joinNetwork')}
          </button>
        ) : (
          <div className="flex flex-wrap items-center justify-end gap-2">
            <div className="border-[3px] border-[#008000] text-[#008000] px-3 py-1 font-sans text-[11px] font-bold uppercase tracking-[1px]">
              {t('connected')}
            </div>
            <button
              onClick={handleLeave}
              disabled={updatingNetwork}
              className="border-[3px] border-[#FF0000] bg-[#FFFFFF] text-[#FF0000] px-4 py-2 sm:px-6 sm:py-2 font-bold text-sm sm:text-base uppercase hover:bg-[#FF0000] hover:text-[#FFFFFF] transition-none cursor-pointer disabled:opacity-50"
            >
              {t('leaveNetwork')}
            </button>
          </div>
        )}
      </div>

      <div className="relative w-full max-w-[300px] aspect-square border-[3px] border-[#000000] rounded-full flex items-center justify-center overflow-hidden bg-[#F9F9F9]">
        {/* Radar Rings */}
        <div className="absolute w-[66%] h-[66%] border-[2px] border-dashed border-[#CCCCCC] rounded-full"></div>
        <div className="absolute w-[33%] h-[33%] border-[2px] border-dashed border-[#CCCCCC] rounded-full"></div>
        
        {/* Scanning Sweep */}
        <div className="absolute w-[50%] h-[50%] origin-bottom-right bg-gradient-to-br from-transparent to-[#000000]/10 animate-spin" style={{ top: 0, left: 0, animationDuration: '3s' }}></div>

        {/* Center Node (Local) */}
        <div className="absolute z-10 w-4 h-4 bg-[#000000] rounded-full border-[2px] border-[#FFFFFF] shadow-none"></div>
        <div className="absolute z-10 mt-10 text-[10px] sm:text-xs font-mono bg-[#FFFFFF] px-1 border-[1px] border-[#000000] uppercase">{t('local')}</div>

        {/* Peers */}
        {peers.map((peer) => {
          // Calculate a random fixed position for each peer based on their string hash
          const hash = peer.id.split('').reduce((a, b) => a + b.charCodeAt(0), 0);
          const angle = (hash % 360) * (Math.PI / 180);
          const distance = 50 + (hash % 80); // Distance between 50 and 130
          
          const x = Math.cos(angle) * distance;
          const y = Math.sin(angle) * distance;

          return (
            <React.Fragment key={peer.id}>
              <div 
                className="absolute z-10 w-3 h-3 bg-[#FF0000] rounded-full border-[2px] border-[#FFFFFF] animate-pulse"
                style={{ transform: `translate(${x}px, ${y}px)` }}
              ></div>
              <div 
                className="absolute z-10 text-[10px] sm:text-xs font-mono bg-[#FFFFFF] px-1 border-[1px] border-[#000000]"
                style={{ transform: `translate(${x}px, ${y + 20}px)` }}
              >
                {peer.display_name || peer.id}
              </div>
            </React.Fragment>
          );
        })}

        {peers.length === 0 && (
          <div className="absolute bottom-6 text-xs sm:text-sm font-mono text-[#666666]">{t('scanningForSignals')}</div>
        )}
      </div>

      {peers.length > 0 && (
        <div className="w-full mt-8 border-[3px] border-[#000000] overflow-x-auto">
          <table className="w-full text-left min-w-[780px]">
            <thead className="bg-[#000000] text-[#FFFFFF]">
              <tr>
                <th className="p-3 font-display uppercase text-xs">{t('deviceId')}</th>
                <th className="p-3 font-display uppercase text-xs">{t('syncStatus')}</th>
                <th className="p-3 font-display uppercase text-xs">{t('tokens24h')}</th>
                <th className="p-3 font-display uppercase text-xs">{t('totalTokens')}</th>
                <th className="p-3 font-display uppercase text-xs">{t('sourceBreakdown')}</th>
                <th className="p-3 font-display uppercase text-xs">{t('lanEndpoint')}</th>
              </tr>
            </thead>
            <tbody>
              {peers.map((peer) => (
                <tr key={peer.id} className="border-b-[3px] border-[#000000] last:border-b-0">
                  <td className="p-3 font-mono">
                    <div className="font-bold">{peer.display_name || peer.id}</div>
                    <div className="text-xs text-[#666666]">{peer.id}</div>
                  </td>
                  <td className={`p-3 font-mono uppercase ${statusClass(peer.sync_status)}`}>
                    <div>{peer.sync_status || 'pending'}</div>
                    {peer.sync_error && <div className="normal-case text-xs text-[#FF0000] max-w-[220px] truncate" title={peer.sync_error}>{peer.sync_error}</div>}
                  </td>
                  <td className="p-3 font-mono">{fmt(peer.tokens_24h)}</td>
                  <td className="p-3 font-mono">{fmt(peer.tokens_total)}</td>
                  <td className="p-3 font-mono text-xs">
                    {Array.isArray(peer.sources) && peer.sources.length > 0 ? (
                      <div className="flex flex-col gap-1 min-w-[150px]">
                        {peer.sources.map((source) => (
                          <div key={source.source} className="flex items-center justify-between gap-3 whitespace-nowrap">
                            <span className="font-bold">{source.source}</span>
                            <span>{fmt(source.tokens_total)}</span>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <span>-</span>
                    )}
                  </td>
                  <td className="p-3 font-mono text-xs">{peer.ip || '-'}{peer.http_port ? `:${peer.http_port}` : ''}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
