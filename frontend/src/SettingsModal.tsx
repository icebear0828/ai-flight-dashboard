import React, { useEffect, useState } from 'react';
import PricingTab from './components/SettingsModal/PricingTab';
import SystemConfigTab from './components/SettingsModal/SystemConfigTab';

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
                <PricingTab
                  pricing={pricing}
                  newModelName={newModelName}
                  setNewModelName={setNewModelName}
                  handleAddModel={handleAddModel}
                  handleUpdatePrice={handleUpdatePrice}
                />
              )}

              {/* --- SYSTEM CONFIG TAB --- */}
              {activeTab === 'system' && (
                <SystemConfigTab
                  config={config}
                  devices={devices}
                  newPath={newPath}
                  setNewPath={setNewPath}
                  handleAddPath={handleAddPath}
                  handleRemovePath={handleRemovePath}
                  editingDevice={editingDevice}
                  setEditingDevice={setEditingDevice}
                  editAliasName={editAliasName}
                  setEditAliasName={setEditAliasName}
                  handleSaveAlias={handleSaveAlias}
                />
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
