import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
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
  enable_lan?: boolean;
}

export interface DeviceInfo {
  id: string;
  display_name: string;
}

interface SettingsModalProps {
  onClose: () => void;
}

export default function SettingsModal({ onClose }: SettingsModalProps) {
  const { t } = useTranslation();
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

  const handleRemoveModel = (index: number) => {
    const newPricing = [...pricing];
    newPricing.splice(index, 1);
    setPricing(newPricing);
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

  const handleToggleLAN = () => {
    setConfig({ ...config, enable_lan: !(config.enable_lan !== false) });
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
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 md:p-6 bg-[#000000]/80 overflow-y-auto" style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}>
      <div className="bg-[#FFFFFF] text-[#000000] border-[5px] border-[#000000] w-full max-w-5xl flex flex-col max-h-full">
        {/* Header */}
        <div className="flex justify-between items-center p-4 md:p-6 border-b-[5px] border-[#000000] bg-[#F0F0F0]">
          <h2 className="font-display text-2xl sm:text-3xl uppercase leading-none">{t('settingsPanel')}</h2>
          <button 
            onClick={onClose}
            className="font-mono text-xl sm:text-2xl border-[3px] border-[#000000] w-10 h-10 flex items-center justify-center hover:bg-[#000000] hover:text-[#FFFFFF] transition-none"
          >
            ×
          </button>
        </div>

        {/* Tab Navigation */}
        <div className="flex border-b-[5px] border-[#000000] bg-[#F0F0F0] overflow-x-auto">
          <button 
            onClick={() => setActiveTab('pricing')} 
            className={`flex-1 py-3 px-4 whitespace-nowrap font-display text-lg sm:text-xl uppercase border-r-[5px] border-[#000000] transition-none ${activeTab === 'pricing' ? 'bg-[#000000] text-[#FFFFFF]' : 'hover:bg-[#E0E0E0]'}`}
          >
            {t('modelPricing')}
          </button>
          <button 
            onClick={() => setActiveTab('system')} 
            className={`flex-1 py-3 px-4 whitespace-nowrap font-display text-lg sm:text-xl uppercase transition-none ${activeTab === 'system' ? 'bg-[#000000] text-[#FFFFFF]' : 'hover:bg-[#E0E0E0]'}`}
          >
            {t('systemConfig')}
          </button>
        </div>

        {/* Content */}
        <div className="p-4 md:p-6 overflow-y-auto flex-1 font-mono text-sm sm:text-base">
          {loading ? (
            <div className="p-10 text-center font-display text-xl sm:text-2xl">{t('loading')}</div>
          ) : (
            <>
              {/* --- PRICING TAB --- */}
              {activeTab === 'pricing' && (
                <PricingTab
                  pricing={pricing}
                  newModelName={newModelName}
                  setNewModelName={setNewModelName}
                  handleAddModel={handleAddModel}
                  handleRemoveModel={handleRemoveModel}
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
                  handleToggleLAN={handleToggleLAN}
                />
              )}
            </>
          )}
        </div>

        {/* Footer */}
        <div className="p-4 md:p-6 border-t-[5px] border-[#000000] flex justify-end gap-4 bg-[#F0F0F0] flex-wrap">
          <button 
            onClick={onClose}
            className="border-[3px] border-[#000000] px-6 py-3 md:px-8 font-bold uppercase hover:bg-[#CCCCCC] transition-none w-full sm:w-auto"
          >
            {t('close')}
          </button>
          <button 
            onClick={handleSave}
            disabled={saving || loading}
            className="border-[3px] border-[#000000] bg-[#000000] text-[#FFFFFF] px-6 py-3 md:px-8 font-bold uppercase hover:bg-[#333333] transition-none disabled:opacity-50 w-full sm:w-auto"
          >
            {saving ? t('saving') : t('saveConfigPricing')}
          </button>
        </div>
      </div>
    </div>
  );
}
