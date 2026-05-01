import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';

export default function Radar() {
  const { t } = useTranslation();
  const [peers, setPeers] = useState<string[]>([]);
  const [joined, setJoined] = useState(false);

  useEffect(() => {
    const fetchPeers = async () => {
      try {
        const res = await fetch('/api/lan/scan');
        const data = await res.json();
        if (data.peers) {
          setPeers(data.peers);
        }
      } catch (e) {
        console.error(e);
      }
    };
    
    fetchPeers();
    const interval = setInterval(fetchPeers, 3000);
    return () => clearInterval(interval);
  }, []);

  const handleJoin = async () => {
    try {
      await fetch('/api/lan/join', { method: 'POST' });
      setJoined(true);
    } catch (e) {
      console.error(e);
    }
  };

  return (
    <div className="border-[5px] border-[#000000] p-4 sm:p-6 md:p-10 bg-[#FFFFFF] flex flex-col items-center mb-16 md:mb-20">
      <div className="w-full flex justify-between items-center border-b-[3px] border-[#000000] pb-4 mb-8">
        <h2 className="font-display text-2xl sm:text-3xl uppercase">{t('lanRadar')}</h2>
        {!joined ? (
          <button 
            onClick={handleJoin}
            className="border-[3px] border-[#000000] bg-[#000000] text-[#FFFFFF] px-4 py-2 sm:px-6 sm:py-2 font-bold text-sm sm:text-base uppercase hover:bg-[#333333] transition-none cursor-pointer"
          >
            {t('joinNetwork')}
          </button>
        ) : (
          <div className="border-[3px] border-[#008000] text-[#008000] px-3 py-1 font-sans text-[11px] font-bold uppercase tracking-[1px]">
            {t('connected')}
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
        {peers.map((peer, i) => {
          // Calculate a random fixed position for each peer based on their string hash
          const hash = peer.split('').reduce((a, b) => a + b.charCodeAt(0), 0);
          const angle = (hash % 360) * (Math.PI / 180);
          const distance = 50 + (hash % 80); // Distance between 50 and 130
          
          const x = Math.cos(angle) * distance;
          const y = Math.sin(angle) * distance;

          return (
            <React.Fragment key={peer}>
              <div 
                className="absolute z-10 w-3 h-3 bg-[#FF0000] rounded-full border-[2px] border-[#FFFFFF] animate-pulse"
                style={{ transform: `translate(${x}px, ${y}px)` }}
              ></div>
              <div 
                className="absolute z-10 text-[10px] sm:text-xs font-mono bg-[#FFFFFF] px-1 border-[1px] border-[#000000]"
                style={{ transform: `translate(${x}px, ${y + 20}px)` }}
              >
                {peer}
              </div>
            </React.Fragment>
          );
        })}

        {peers.length === 0 && (
          <div className="absolute bottom-6 text-xs sm:text-sm font-mono text-[#666666]">{t('scanningForSignals')}</div>
        )}
      </div>
    </div>
  );
}
