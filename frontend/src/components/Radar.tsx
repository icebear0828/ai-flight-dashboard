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
    <div className="border-[5px] border-[#000000] p-[24px] bg-[#FFFFFF] flex flex-col items-center mb-[80px]">
      <div className="w-full flex justify-between items-center border-b-[3px] border-[#000000] pb-[16px] mb-[32px]">
        <h2 className="font-display text-[32px] uppercase">LAN Radar</h2>
        {!joined ? (
          <button 
            onClick={handleJoin}
            className="border-[3px] border-[#000000] bg-[#000000] text-[#FFFFFF] px-[24px] py-[8px] font-bold uppercase hover:bg-[#333333] transition-none cursor-pointer"
          >
            JOIN NETWORK
          </button>
        ) : (
          <div className="border-[3px] border-[#008000] text-[#008000] px-[12px] py-[4px] font-sans text-[11px] font-bold uppercase tracking-[1px]">
            CONNECTED
          </div>
        )}
      </div>

      <div className="relative w-[300px] h-[300px] border-[3px] border-[#000000] rounded-full flex items-center justify-center overflow-hidden bg-[#F9F9F9]">
        {/* Radar Rings */}
        <div className="absolute w-[200px] h-[200px] border-[2px] border-dashed border-[#CCCCCC] rounded-full"></div>
        <div className="absolute w-[100px] h-[100px] border-[2px] border-dashed border-[#CCCCCC] rounded-full"></div>
        
        {/* Scanning Sweep */}
        <div className="absolute w-[150px] h-[150px] origin-bottom-right bg-gradient-to-br from-transparent to-[#000000]/10 animate-spin" style={{ top: 0, left: 0, animationDuration: '3s' }}></div>

        {/* Center Node (Local) */}
        <div className="absolute z-10 w-[16px] h-[16px] bg-[#000000] rounded-full border-[2px] border-[#FFFFFF] shadow-none"></div>
        <div className="absolute z-10 mt-[40px] text-[10px] font-mono bg-[#FFFFFF] px-[4px] border-[1px] border-[#000000]">LOCAL</div>

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
                className="absolute z-10 w-[12px] h-[12px] bg-[#FF0000] rounded-full border-[2px] border-[#FFFFFF] animate-pulse"
                style={{ transform: `translate(${x}px, ${y}px)` }}
              ></div>
              <div 
                className="absolute z-10 text-[10px] font-mono bg-[#FFFFFF] px-[4px] border-[1px] border-[#000000]"
                style={{ transform: `translate(${x}px, ${y + 20}px)` }}
              >
                {peer}
              </div>
            </React.Fragment>
          );
        })}

        {peers.length === 0 && (
          <div className="absolute bottom-[20px] text-[12px] font-mono text-[#666666]">Scanning for signals...</div>
        )}
      </div>
    </div>
  );
}
