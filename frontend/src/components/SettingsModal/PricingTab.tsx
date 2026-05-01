import React from 'react';
import { PricingEntry } from '../../SettingsModal';

interface PricingTabProps {
  pricing: PricingEntry[];
  newModelName: string;
  setNewModelName: (name: string) => void;
  handleAddModel: () => void;
  handleUpdatePrice: (index: number, field: keyof PricingEntry, value: string) => void;
}

export default function PricingTab({
  pricing,
  newModelName,
  setNewModelName,
  handleAddModel,
  handleUpdatePrice,
}: PricingTabProps) {
  return (
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
  );
}
