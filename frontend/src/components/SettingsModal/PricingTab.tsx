import React from 'react';
import { useTranslation } from 'react-i18next';
import { PricingEntry } from '../../SettingsModal';

interface PricingTabProps {
  pricing: PricingEntry[];
  newModelName: string;
  setNewModelName: (name: string) => void;
  handleAddModel: () => void;
  handleRemoveModel: (index: number) => void;
  handleUpdatePrice: (index: number, field: keyof PricingEntry, value: string) => void;
}

export default function PricingTab({
  pricing,
  newModelName,
  setNewModelName,
  handleAddModel,
  handleRemoveModel,
  handleUpdatePrice,
}: PricingTabProps) {
  const { t } = useTranslation();
  return (
    <div className="mb-8">
      {/* Add New Model Form */}
      <div className="flex flex-col md:flex-row gap-4 mb-6 items-start md:items-end p-4 border-[3px] border-[#000000] bg-[#F9F9F9]">
        <div className="flex-1 w-full">
          <label className="block mb-2 font-bold">{t('newModelIdentifier')}</label>
          <input 
            type="text" 
            value={newModelName}
            onChange={(e) => setNewModelName(e.target.value)}
            placeholder="e.g. claude-3-5-sonnet-custom"
            className="w-full border-[3px] border-[#000000] px-3 py-2 outline-none focus:bg-[#000000] focus:text-[#FFFFFF] transition-none"
          />
        </div>
        <button 
          onClick={handleAddModel}
          className="border-[3px] border-[#000000] px-6 py-2 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] transition-none whitespace-nowrap w-full md:w-auto mt-4 md:mt-0"
        >
          {t('addModel')}
        </button>
      </div>

      {/* Pricing Table */}
      <div className="overflow-x-auto border-[3px] border-[#000000]">
        <table className="w-full text-left min-w-[600px]">
          <thead className="bg-[#000000] text-[#FFFFFF]">
            <tr>
              <th className="p-3 font-display uppercase">{t('model')}</th>
              <th className="p-3 font-display uppercase min-w-[120px]">{t('input')}</th>
              <th className="p-3 font-display uppercase min-w-[120px]">{t('cachedRead')}</th>
              <th className="p-3 font-display uppercase min-w-[120px]">{t('cachedWrite')}</th>
              <th className="p-3 font-display uppercase min-w-[120px]">{t('output')}</th>
              <th className="p-3 font-display uppercase w-24 text-center"></th>
            </tr>
          </thead>
          <tbody>
            {pricing.map((p, i) => (
              <tr key={p.model} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#F0F0F0]">
                <td className="p-3 font-bold">{p.model}</td>
                <td className="p-2">
                  <input 
                    type="number" 
                    step="0.01" 
                    value={p.input_price_per_m} 
                    onChange={(e) => handleUpdatePrice(i, 'input_price_per_m', e.target.value)}
                    className="w-full border-[2px] border-[#000000] px-2 py-1 outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                  />
                </td>
                <td className="p-2">
                  <input 
                    type="number" 
                    step="0.01" 
                    value={p.cached_price_per_m} 
                    onChange={(e) => handleUpdatePrice(i, 'cached_price_per_m', e.target.value)}
                    className="w-full border-[2px] border-[#000000] px-2 py-1 outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                  />
                </td>
                <td className="p-2">
                  <input 
                    type="number" 
                    step="0.01" 
                    value={p.cache_creation_price_per_m} 
                    onChange={(e) => handleUpdatePrice(i, 'cache_creation_price_per_m', e.target.value)}
                    className="w-full border-[2px] border-[#000000] px-2 py-1 outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                  />
                </td>
                <td className="p-2">
                  <input 
                    type="number" 
                    step="0.01" 
                    value={p.output_price_per_m} 
                    onChange={(e) => handleUpdatePrice(i, 'output_price_per_m', e.target.value)}
                    className="w-full border-[2px] border-[#000000] px-2 py-1 outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                  />
                </td>
                <td className="p-2 text-center">
                  <button 
                    onClick={() => handleRemoveModel(i)}
                    className="bg-[#FF0000] text-[#FFFFFF] font-bold px-3 py-1 border-[2px] border-[#FF0000] hover:opacity-80 whitespace-nowrap"
                  >
                    {t('delete')}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
