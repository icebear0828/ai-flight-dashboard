import React, { useEffect, useState } from 'react';

export interface PricingEntry {
  model: string;
  input_price_per_m: number;
  cached_price_per_m: number;
  cache_creation_price_per_m: number;
  output_price_per_m: number;
}

export interface AppConfig {
  auto_start: boolean;
  extra_watch_dirs: string[];
}

export interface DeviceInfo {
  id: string;
  display_name: string;
}

interface SettingsModalProps {
  onClose: () => void;
}

export default function SettingsModal({ onClose }: SettingsModalProps) {
  const [activeTab, setActiveTab] = useState<'pricing' | 'system'>('pricing');
  
  const [pricing, setPricing] = useState<PricingEntry[]>([]);
  const [config, setConfig] = useState<AppConfig>({ auto_start: false, extra_watch_dirs: [] });
  const [devices, setDevices] = useState<DeviceInfo[]>([]);
  
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  
  const [newModelName, setNewModelName] = useState('');
  const [newPath, setNewPath] = useState('');
  
  // Temporary state for inline device editing
  const [editingDevice, setEditingDevice] = useState<string | null>(null);
  const [editAliasName, setEditAliasName] = useState('');

  useEffect(() => {
    Promise.all([
      fetch('/api/pricing').then(res => res.json()),
      fetch('/api/config').then(res => res.json()),
      fetch('/api/stats?device=all').then(res => res.json())
    ]).then(([pricingData, configData, statsData]) => {
      if (Array.isArray(pricingData)) setPricing(pricingData);
      if (configData) setConfig(configData);
      if (statsData && Array.isArray(statsData.devices)) setDevices(statsData.devices);
      setLoading(false);
    }).catch(err => {
      console.error("Failed to load data", err);
      setLoading(false);
    });
  }, []);

  const handleUpdatePrice = (index: number, field: keyof PricingEntry, value: string) => {
    const val = parseFloat(value) || 0;
    const newPricing = [...pricing];
    newPricing[index] = { ...newPricing[index], [field]: val };
    setPricing(newPricing);
  };

  const handleAddModel = () => {
    if (!newModelName.trim()) return;
    if (pricing.some(p => p.model === newModelName)) {
      alert("Model already exists.");
      return;
    }
    setPricing([
      {
        model: newModelName.trim(),
        input_price_per_m: 0,
        cached_price_per_m: 0,
        cache_creation_price_per_m: 0,
        output_price_per_m: 0
      },
      ...pricing
    ]);
    setNewModelName('');
  };

  const handleAddPath = () => {
    if (!newPath.trim()) return;
    if (config.extra_watch_dirs.includes(newPath.trim())) {
      alert("Path already exists.");
      return;
    }
    setConfig({ ...config, extra_watch_dirs: [...(config.extra_watch_dirs || []), newPath.trim()] });
    setNewPath('');
  };

  const handleRemovePath = (index: number) => {
    const newDirs = [...(config.extra_watch_dirs || [])];
    newDirs.splice(index, 1);
    setConfig({ ...config, extra_watch_dirs: newDirs });
  };

  const handleSaveAlias = async (deviceId: string) => {
    if (!editAliasName.trim()) return;
    try {
      await fetch('/api/device-alias', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ device_id: deviceId, display_name: editAliasName.trim() })
      });
      setDevices(devices.map(d => d.id === deviceId ? { ...d, display_name: editAliasName.trim() } : d));
      setEditingDevice(null);
    } catch (err) {
      console.error("Failed to save alias", err);
      alert("Failed to save device alias.");
    }
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await Promise.all([
        fetch('/api/pricing', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(pricing)
        }),
        fetch('/api/config', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(config)
        })
      ]);
      onClose();
    } catch (err) {
      console.error("Failed to save", err);
      alert("Failed to save settings.");
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-[24px] bg-[#000000]/80 overflow-y-auto" style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}>
      <div className="bg-[#FFFFFF] text-[#000000] border-[5px] border-[#000000] w-full max-w-[1000px] flex flex-col max-h-full">
        {/* Header */}
        <div className="flex justify-between items-center p-[24px] border-b-[5px] border-[#000000] bg-[#F0F0F0]">
          <h2 className="font-display text-[32px] uppercase leading-none">Settings Panel</h2>
          <button 
            onClick={onClose}
            className="font-mono text-[24px] border-[3px] border-[#000000] w-[40px] h-[40px] flex items-center justify-center hover:bg-[#000000] hover:text-[#FFFFFF] transition-none"
          >
            ×
          </button>
        </div>

        {/* Tab Navigation */}
        <div className="flex border-b-[5px] border-[#000000] bg-[#F0F0F0]">
          <button 
            onClick={() => setActiveTab('pricing')} 
            className={`flex-1 py-[12px] font-display text-[20px] uppercase border-r-[5px] border-[#000000] transition-none ${activeTab === 'pricing' ? 'bg-[#000000] text-[#FFFFFF]' : 'hover:bg-[#E0E0E0]'}`}
          >
            Model Pricing
          </button>
          <button 
            onClick={() => setActiveTab('system')} 
            className={`flex-1 py-[12px] font-display text-[20px] uppercase transition-none ${activeTab === 'system' ? 'bg-[#000000] text-[#FFFFFF]' : 'hover:bg-[#E0E0E0]'}`}
          >
            System Config
          </button>
        </div>

        {/* Content */}
        <div className="p-[24px] overflow-y-auto flex-1 font-mono text-[14px]">
          {loading ? (
            <div className="p-[40px] text-center font-display text-[24px]">LOADING...</div>
          ) : (
            <>
              {/* --- PRICING TAB --- */}
              {activeTab === 'pricing' && (
                <div className="mb-[32px]">
                  {/* Add New Model Form */}
                  <div className="flex flex-col md:flex-row gap-[16px] mb-[24px] items-start md:items-end p-[16px] border-[3px] border-[#000000] bg-[#F9F9F9]">
                    <div className="flex-1">
                      <label className="block mb-[8px] font-bold">New Model Identifier</label>
                      <input 
                        type="text" 
                        value={newModelName}
                        onChange={(e) => setNewModelName(e.target.value)}
                        placeholder="e.g. claude-3-5-sonnet-custom"
                        className="w-full border-[3px] border-[#000000] px-[12px] py-[8px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF] transition-none"
                      />
                    </div>
                    <button 
                      onClick={handleAddModel}
                      className="border-[3px] border-[#000000] px-[24px] py-[8px] font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] transition-none whitespace-nowrap"
                    >
                      + Add Model
                    </button>
                  </div>

                  {/* Pricing Table */}
                  <div className="overflow-x-auto border-[3px] border-[#000000]">
                    <table className="w-full text-left">
                      <thead className="bg-[#000000] text-[#FFFFFF]">
                        <tr>
                          <th className="p-[12px] font-display uppercase">Model</th>
                          <th className="p-[12px] font-display uppercase w-[150px]">Input ($)</th>
                          <th className="p-[12px] font-display uppercase w-[150px]">Cached Read ($)</th>
                          <th className="p-[12px] font-display uppercase w-[150px]">Cached Write ($)</th>
                          <th className="p-[12px] font-display uppercase w-[150px]">Output ($)</th>
                        </tr>
                      </thead>
                      <tbody>
                        {pricing.map((p, i) => (
                          <tr key={p.model} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#F0F0F0]">
                            <td className="p-[12px] font-bold">{p.model}</td>
                            <td className="p-[8px]">
                              <input 
                                type="number" 
                                step="0.01" 
                                value={p.input_price_per_m} 
                                onChange={(e) => handleUpdatePrice(i, 'input_price_per_m', e.target.value)}
                                className="w-full border-[2px] border-[#000000] px-[8px] py-[4px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                              />
                            </td>
                            <td className="p-[8px]">
                              <input 
                                type="number" 
                                step="0.01" 
                                value={p.cached_price_per_m} 
                                onChange={(e) => handleUpdatePrice(i, 'cached_price_per_m', e.target.value)}
                                className="w-full border-[2px] border-[#000000] px-[8px] py-[4px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                              />
                            </td>
                            <td className="p-[8px]">
                              <input 
                                type="number" 
                                step="0.01" 
                                value={p.cache_creation_price_per_m} 
                                onChange={(e) => handleUpdatePrice(i, 'cache_creation_price_per_m', e.target.value)}
                                className="w-full border-[2px] border-[#000000] px-[8px] py-[4px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                              />
                            </td>
                            <td className="p-[8px]">
                              <input 
                                type="number" 
                                step="0.01" 
                                value={p.output_price_per_m} 
                                onChange={(e) => handleUpdatePrice(i, 'output_price_per_m', e.target.value)}
                                className="w-full border-[2px] border-[#000000] px-[8px] py-[4px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                              />
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                </div>
              )}

              {/* --- SYSTEM CONFIG TAB --- */}
              {activeTab === 'system' && (
                <div className="flex flex-col gap-[32px] mb-[32px]">
                  
                  {/* Device Aliases Section */}
                  <section>
                    <h3 className="font-display text-[24px] uppercase border-b-[3px] border-[#000000] pb-[8px] mb-[16px]">Device Aliases</h3>
                    <div className="border-[3px] border-[#000000]">
                      <table className="w-full text-left">
                        <thead className="bg-[#000000] text-[#FFFFFF]">
                          <tr>
                            <th className="p-[12px] font-display uppercase w-1/3">Device ID</th>
                            <th className="p-[12px] font-display uppercase w-2/3">Display Name</th>
                          </tr>
                        </thead>
                        <tbody>
                          {devices.map((d) => (
                            <tr key={d.id} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#F0F0F0]">
                              <td className="p-[12px] font-mono text-[#666666]">{d.id}</td>
                              <td className="p-[12px]">
                                {editingDevice === d.id ? (
                                  <div className="flex gap-[8px]">
                                    <input 
                                      type="text" 
                                      value={editAliasName} 
                                      onChange={e => setEditAliasName(e.target.value)}
                                      className="flex-1 border-[2px] border-[#000000] px-[8px] py-[4px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                                    />
                                    <button 
                                      onClick={() => handleSaveAlias(d.id)}
                                      className="bg-[#008000] text-[#FFFFFF] border-[2px] border-[#008000] px-[12px] font-bold hover:opacity-80"
                                    >
                                      SAVE
                                    </button>
                                    <button 
                                      onClick={() => setEditingDevice(null)}
                                      className="bg-[#FF0000] text-[#FFFFFF] border-[2px] border-[#FF0000] px-[12px] font-bold hover:opacity-80"
                                    >
                                      CANCEL
                                    </button>
                                  </div>
                                ) : (
                                  <div className="flex justify-between items-center group">
                                    <span className="font-bold">{d.display_name}</span>
                                    <button 
                                      onClick={() => { setEditingDevice(d.id); setEditAliasName(d.display_name === d.id ? '' : d.display_name); }}
                                      className="opacity-0 group-hover:opacity-100 underline text-[#0000FF]"
                                    >
                                      EDIT
                                    </button>
                                  </div>
                                )}
                              </td>
                            </tr>
                          ))}
                          {devices.length === 0 && (
                            <tr><td colSpan={2} className="p-[12px] text-center italic text-[#666666]">No devices found in local database.</td></tr>
                          )}
                        </tbody>
                      </table>
                    </div>
                  </section>

                  {/* Log Directories Section */}
                  <section>
                    <h3 className="font-display text-[24px] uppercase border-b-[3px] border-[#000000] pb-[8px] mb-[16px]">Extra Watch Directories</h3>
                    <p className="mb-[16px] text-[#666666]">Add custom file paths for the dashboard to scan for JSONL logs. Defaults (Claude/Gemini temp dirs) are always scanned.</p>
                    
                    <ul className="mb-[16px] flex flex-col gap-[8px]">
                      {(config.extra_watch_dirs || []).map((dir, idx) => (
                        <li key={idx} className="flex justify-between items-center border-[3px] border-[#000000] bg-[#F9F9F9] p-[12px]">
                          <span className="font-mono truncate">{dir}</span>
                          <button 
                            onClick={() => handleRemovePath(idx)}
                            className="bg-[#FF0000] text-[#FFFFFF] font-bold px-[12px] py-[4px] border-[2px] border-[#FF0000] hover:opacity-80"
                          >
                            REMOVE
                          </button>
                        </li>
                      ))}
                      {(config.extra_watch_dirs || []).length === 0 && (
                        <li className="italic text-[#666666] border-[3px] border-dashed border-[#CCCCCC] p-[12px] text-center">No extra directories configured.</li>
                      )}
                    </ul>

                    <div className="flex gap-[8px]">
                      <input 
                        type="text" 
                        value={newPath}
                        onChange={(e) => setNewPath(e.target.value)}
                        placeholder="Absolute path e.g. /Users/name/logs"
                        className="flex-1 border-[3px] border-[#000000] px-[12px] py-[8px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF] transition-none"
                      />
                      <button 
                        onClick={handleAddPath}
                        className="border-[3px] border-[#000000] px-[24px] py-[8px] font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] transition-none whitespace-nowrap"
                      >
                        + ADD PATH
                      </button>
                    </div>
                  </section>

                </div>
              )}
            </>
          )}
        </div>

        {/* Footer */}
        <div className="p-[24px] border-t-[5px] border-[#000000] flex justify-end gap-[16px] bg-[#F0F0F0]">
          <button 
            onClick={onClose}
            className="border-[3px] border-[#000000] px-[32px] py-[12px] font-bold uppercase hover:bg-[#CCCCCC] transition-none"
          >
            Close
          </button>
          <button 
            onClick={handleSave}
            disabled={saving || loading}
            className="border-[3px] border-[#000000] bg-[#000000] text-[#FFFFFF] px-[32px] py-[12px] font-bold uppercase hover:bg-[#333333] transition-none disabled:opacity-50"
          >
            {saving ? 'SAVING...' : 'SAVE CONFIG & PRICING'}
          </button>
        </div>
      </div>
    </div>
  );
}
