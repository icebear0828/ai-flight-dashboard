import React, { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import DeviceManagementTab from './components/SettingsModal/DeviceManagementTab';
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

export interface DeviceSummary {
  id: string;
  display_name: string;
  events: number;
  input_tokens: number;
  cached_tokens: number;
  cache_creation_tokens: number;
  output_tokens: number;
  total_cost: number;
  first_seen?: string;
  last_seen?: string;
}

interface SettingsModalProps {
  onClose: () => void;
}

export default function SettingsModal({ onClose }: SettingsModalProps) {
  const { t } = useTranslation();
  const [activeTab, setActiveTab] = useState<'pricing' | 'devices' | 'system'>('pricing');
  
  const [pricing, setPricing] = useState<PricingEntry[]>([]);
  const [config, setConfig] = useState<AppConfig>({ auto_start: false, extra_watch_dirs: [] });
  const [devices, setDevices] = useState<DeviceSummary[]>([]);
  
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  
  const [newModelName, setNewModelName] = useState('');
  const [newPath, setNewPath] = useState('');
  const [newDeviceId, setNewDeviceId] = useState('');
  const [newDeviceName, setNewDeviceName] = useState('');
  
  // Temporary state for inline device editing
  const [editingDevice, setEditingDevice] = useState<string | null>(null);
  const [editAliasName, setEditAliasName] = useState('');
  const [deletingDevice, setDeletingDevice] = useState<string | null>(null);

  const loadDevices = async () => {
    const res = await fetch('/api/devices');
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    const data = await res.json() as DeviceSummary[];
    if (Array.isArray(data)) setDevices(data);
  };

  useEffect(() => {
    Promise.all([
      fetch('/api/pricing').then(res => res.json()),
      fetch('/api/config').then(res => res.json()),
      fetch('/api/devices').then(res => res.json())
    ]).then(([pricingData, configData, devicesData]) => {
      if (Array.isArray(pricingData)) setPricing(pricingData);
      if (configData) {
        setConfig({
          ...configData,
          extra_watch_dirs: Array.isArray(configData.extra_watch_dirs) ? configData.extra_watch_dirs : [],
        });
      }
      if (Array.isArray(devicesData)) setDevices(devicesData);
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
    const dirs = config.extra_watch_dirs || [];
    if (dirs.includes(newPath.trim())) {
      alert("Path already exists.");
      return;
    }
    setConfig({ ...config, extra_watch_dirs: [...dirs, newPath.trim()] });
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

  const saveAlias = async (deviceId: string, displayName: string) => {
    const id = deviceId.trim();
    const name = displayName.trim();
    if (!id || !name) return;
    const res = await fetch('/api/device-alias', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ device_id: id, display_name: name })
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
  };

  const handleAddAlias = async () => {
    try {
      await saveAlias(newDeviceId, newDeviceName);
      setNewDeviceId('');
      setNewDeviceName('');
      await loadDevices();
    } catch (err) {
      console.error("Failed to add alias", err);
      alert("Failed to add device alias.");
    }
  };

  const handleSaveAlias = async (deviceId: string) => {
    try {
      await saveAlias(deviceId, editAliasName);
      setEditingDevice(null);
      await loadDevices();
    } catch (err) {
      console.error("Failed to save alias", err);
      alert("Failed to save device alias.");
    }
  };

  const handleDeleteAlias = async (deviceId: string) => {
    try {
      const res = await fetch(`/api/device-alias?device_id=${encodeURIComponent(deviceId)}`, { method: 'DELETE' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      await loadDevices();
    } catch (err) {
      console.error("Failed to clear alias", err);
      alert("Failed to clear device alias.");
    }
  };

  const handleSupersedeDevice = async (deviceId: string) => {
    setDeletingDevice(deviceId);
    try {
      const res = await fetch(`/api/devices?device_id=${encodeURIComponent(deviceId)}`, { method: 'DELETE' });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      await loadDevices();
    } catch (err) {
      console.error("Failed to clean device", err);
      alert("Failed to clean device.");
    } finally {
      setDeletingDevice(null);
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
            onClick={() => setActiveTab('devices')}
            className={`flex-1 py-3 px-4 whitespace-nowrap font-display text-lg sm:text-xl uppercase border-r-[5px] border-[#000000] transition-none ${activeTab === 'devices' ? 'bg-[#000000] text-[#FFFFFF]' : 'hover:bg-[#E0E0E0]'}`}
          >
            {t('deviceManagement')}
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

              {/* --- DEVICE MANAGEMENT TAB --- */}
              {activeTab === 'devices' && (
                <DeviceManagementTab
                  devices={devices}
                  newDeviceId={newDeviceId}
                  newDeviceName={newDeviceName}
                  setNewDeviceId={setNewDeviceId}
                  setNewDeviceName={setNewDeviceName}
                  handleAddAlias={handleAddAlias}
                  editingDevice={editingDevice}
                  setEditingDevice={setEditingDevice}
                  editAliasName={editAliasName}
                  setEditAliasName={setEditAliasName}
                  deletingDevice={deletingDevice}
                  handleSaveAlias={handleSaveAlias}
                  handleDeleteAlias={handleDeleteAlias}
                  handleSupersedeDevice={handleSupersedeDevice}
                />
              )}

              {/* --- SYSTEM CONFIG TAB --- */}
              {activeTab === 'system' && (
                <SystemConfigTab
                  config={config}
                  newPath={newPath}
                  setNewPath={setNewPath}
                  handleAddPath={handleAddPath}
                  handleRemovePath={handleRemovePath}
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
