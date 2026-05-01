import React, { useEffect, useState } from 'react';

export interface PricingEntry {
  model: string;
  input_price_per_m: number;
  cached_price_per_m: number;
  cache_creation_price_per_m: number;
  output_price_per_m: number;
}

interface SettingsModalProps {
  onClose: () => void;
}

export default function SettingsModal({ onClose }: SettingsModalProps) {
  const [pricing, setPricing] = useState<PricingEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [newModelName, setNewModelName] = useState('');

  useEffect(() => {
    fetch('/api/pricing')
      .then(res => res.json())
      .then(data => {
        if (Array.isArray(data)) {
          setPricing(data);
        }
        setLoading(false);
      })
      .catch(err => {
        console.error("Failed to load pricing", err);
        setLoading(false);
      });
  }, []);

  const handleUpdate = (index: number, field: keyof PricingEntry, value: string) => {
    const val = parseFloat(value) || 0;
    const newPricing = [...pricing];
    newPricing[index] = { ...newPricing[index], [field]: val };
    setPricing(newPricing);
  };

  const handleSave = async () => {
    setSaving(true);
    try {
      await fetch('/api/pricing', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(pricing)
      });
      onClose();
    } catch (err) {
      console.error("Failed to save pricing", err);
      alert("Failed to save settings.");
    } finally {
      setSaving(false);
    }
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

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-[24px] bg-[#000000]/80 overflow-y-auto" style={{ "--wails-draggable": "no-drag" } as React.CSSProperties}>
      <div className="bg-[#FFFFFF] text-[#000000] border-[5px] border-[#000000] w-full max-w-[1000px] flex flex-col max-h-full">
        {/* Header */}
        <div className="flex justify-between items-center p-[24px] border-b-[5px] border-[#000000] bg-[#F0F0F0]">
          <h2 className="font-display text-[32px] uppercase leading-none">Settings & Pricing</h2>
          <button 
            onClick={onClose}
            className="font-mono text-[24px] border-[3px] border-[#000000] w-[40px] h-[40px] flex items-center justify-center hover:bg-[#000000] hover:text-[#FFFFFF] transition-none"
          >
            ×
          </button>
        </div>

        {/* Content */}
        <div className="p-[24px] overflow-y-auto flex-1 font-mono text-[14px]">
          {loading ? (
            <div className="p-[40px] text-center font-display text-[24px]">LOADING...</div>
          ) : (
            <>
              <div className="mb-[32px]">
                <h3 className="font-display text-[24px] uppercase border-b-[3px] border-[#000000] pb-[8px] mb-[16px]">Model Pricing (USD per 1M Tokens)</h3>
                
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
                              onChange={(e) => handleUpdate(i, 'input_price_per_m', e.target.value)}
                              className="w-full border-[2px] border-[#000000] px-[8px] py-[4px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                            />
                          </td>
                          <td className="p-[8px]">
                            <input 
                              type="number" 
                              step="0.01" 
                              value={p.cached_price_per_m} 
                              onChange={(e) => handleUpdate(i, 'cached_price_per_m', e.target.value)}
                              className="w-full border-[2px] border-[#000000] px-[8px] py-[4px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                            />
                          </td>
                          <td className="p-[8px]">
                            <input 
                              type="number" 
                              step="0.01" 
                              value={p.cache_creation_price_per_m} 
                              onChange={(e) => handleUpdate(i, 'cache_creation_price_per_m', e.target.value)}
                              className="w-full border-[2px] border-[#000000] px-[8px] py-[4px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                            />
                          </td>
                          <td className="p-[8px]">
                            <input 
                              type="number" 
                              step="0.01" 
                              value={p.output_price_per_m} 
                              onChange={(e) => handleUpdate(i, 'output_price_per_m', e.target.value)}
                              className="w-full border-[2px] border-[#000000] px-[8px] py-[4px] outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                            />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            </>
          )}
        </div>

        {/* Footer */}
        <div className="p-[24px] border-t-[5px] border-[#000000] flex justify-end gap-[16px] bg-[#F0F0F0]">
          <button 
            onClick={onClose}
            className="border-[3px] border-[#000000] px-[32px] py-[12px] font-bold uppercase hover:bg-[#CCCCCC] transition-none"
          >
            Cancel
          </button>
          <button 
            onClick={handleSave}
            disabled={saving || loading}
            className="border-[3px] border-[#000000] bg-[#000000] text-[#FFFFFF] px-[32px] py-[12px] font-bold uppercase hover:bg-[#333333] transition-none disabled:opacity-50"
          >
            {saving ? 'SAVING...' : 'SAVE CHANGES'}
          </button>
        </div>
      </div>
    </div>
  );
}
