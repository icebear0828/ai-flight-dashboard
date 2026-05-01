import React, { useEffect, useState } from 'react';

export default function Radar() {
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
    <div className="glass-panel p-[32px] flex flex-col items-center mb-[40px]">
      <div className="w-full flex justify-between items-center border-b border-panel-border pb-[16px] mb-[32px]">
        <div className="flex items-center gap-[12px]">
          <h2 className="font-display text-[24px] md:text-[32px] font-semibold text-white tracking-wide">LAN RADAR</h2>
          <span className="text-neon-cyan/60 text-[14px] font-mono tracking-widest uppercase hidden md:inline-block">TOPOLOGY MAP</span>
        </div>
        {!joined ? (
          <button 
            onClick={handleJoin}
            className="glass-panel border-neon-cyan/50 text-neon-cyan px-[24px] py-[8px] font-bold uppercase hover:bg-neon-cyan/10 transition-colors shadow-[0_0_15px_rgba(0,240,255,0.2)] cursor-pointer"
          >
            JOIN NETWORK
          </button>
        ) : (
          <div className="glass-panel border-neon-green/50 text-neon-green px-[16px] py-[6px] flex items-center gap-[8px] shadow-[0_0_15px_rgba(57,255,20,0.15)]">
            <span className="w-2 h-2 rounded-full bg-neon-green animate-pulse"></span>
            <span className="font-mono text-[12px] font-bold uppercase tracking-[2px]">CONNECTED</span>
          </div>
        )}
      </div>

      <div className="relative w-[300px] h-[300px] md:w-[400px] md:h-[400px] border border-panel-border rounded-full flex items-center justify-center overflow-hidden bg-bg-deep/50 shadow-[0_0_40px_rgba(0,240,255,0.05)]">
        {/* Radar Rings */}
        <div className="absolute w-[250px] h-[250px] md:w-[320px] md:h-[320px] border border-dashed border-neon-cyan/20 rounded-full"></div>
        <div className="absolute w-[150px] h-[150px] md:w-[200px] md:h-[200px] border border-dashed border-neon-cyan/30 rounded-full"></div>
        <div className="absolute w-[50px] h-[50px] md:w-[80px] md:h-[80px] border border-dashed border-neon-cyan/40 rounded-full"></div>
        
        {/* Scanning Sweep */}
        <div className="absolute w-[150px] h-[150px] md:w-[200px] md:h-[200px] origin-bottom-right bg-gradient-to-br from-neon-cyan/0 via-neon-cyan/5 to-neon-cyan/30 animate-spin border-r-2 border-neon-cyan/50 shadow-[0_0_15px_rgba(0,240,255,0.3)]" style={{ top: 0, left: 0, animationDuration: '4s', animationTimingFunction: 'linear' }}></div>

        {/* Center Node (Local) */}
        <div className="absolute z-10 w-[20px] h-[20px] bg-bg-deep rounded-full border-2 border-neon-cyan shadow-[0_0_15px_rgba(0,240,255,0.8)] flex items-center justify-center">
          <div className="w-[8px] h-[8px] bg-neon-cyan rounded-full animate-pulse"></div>
        </div>
        <div className="absolute z-10 mt-[48px] text-[10px] font-mono text-neon-cyan bg-bg-deep/80 backdrop-blur-sm px-[8px] py-[2px] border border-neon-cyan/30 rounded-full tracking-widest shadow-[0_0_10px_rgba(0,240,255,0.2)]">LOCAL</div>

        {/* Crosshairs */}
        <div className="absolute w-full h-[1px] bg-neon-cyan/10"></div>
        <div className="absolute h-full w-[1px] bg-neon-cyan/10"></div>

        {/* Peers */}
        {peers.map((peer, i) => {
          // Calculate a random fixed position for each peer based on their string hash
          const hash = peer.split('').reduce((a, b) => a + b.charCodeAt(0), 0);
          const angle = (hash % 360) * (Math.PI / 180);
          const distance = 80 + (hash % 100); // Distance between 80 and 180
          
          const x = Math.cos(angle) * distance;
          const y = Math.sin(angle) * distance;

          return (
            <React.Fragment key={peer}>
              <div 
                className="absolute z-10 w-[14px] h-[14px] bg-bg-deep rounded-full border-2 border-neon-purple shadow-[0_0_15px_rgba(176,38,255,0.8)] flex items-center justify-center"
                style={{ transform: `translate(${x}px, ${y}px)` }}
              >
                <div className="w-[6px] h-[6px] bg-neon-purple rounded-full animate-ping"></div>
              </div>
              <div 
                className="absolute z-10 text-[10px] font-mono text-neon-purple bg-bg-deep/80 backdrop-blur-sm px-[8px] py-[2px] border border-neon-purple/30 rounded-full tracking-wider shadow-[0_0_10px_rgba(176,38,255,0.2)]"
                style={{ transform: `translate(${x}px, ${y + 24}px)` }}
              >
                {peer}
              </div>
            </React.Fragment>
          );
        })}

        {peers.length === 0 && (
          <div className="absolute bottom-[24px] text-[11px] font-mono text-neon-cyan/50 tracking-widest uppercase animate-pulse">Scanning frequencies...</div>
        )}
      </div>
    </div>
  );
}
