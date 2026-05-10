import React from 'react';
import { useTranslation } from 'react-i18next';
import { DeviceSummary } from '../../SettingsModal';

interface DeviceManagementTabProps {
  devices: DeviceSummary[];
  newDeviceId: string;
  newDeviceName: string;
  setNewDeviceId: (id: string) => void;
  setNewDeviceName: (name: string) => void;
  handleAddAlias: () => void;
  editingDevice: string | null;
  setEditingDevice: (id: string | null) => void;
  editAliasName: string;
  setEditAliasName: (name: string) => void;
  handleSaveAlias: (id: string) => void;
  handleDeleteAlias: (id: string) => void;
  handleSupersedeDevice: (id: string) => void;
}

const formatTokens = (value: number): string => {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`;
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`;
  if (value >= 1_000) return `${(value / 1_000).toFixed(1)}K`;
  return value.toString();
};

const formatDate = (value?: string): string => {
  if (!value || value.startsWith('0001-01-01')) return '-';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return '-';
  return date.toLocaleString();
};

export default function DeviceManagementTab({
  devices,
  newDeviceId,
  newDeviceName,
  setNewDeviceId,
  setNewDeviceName,
  handleAddAlias,
  editingDevice,
  setEditingDevice,
  editAliasName,
  setEditAliasName,
  handleSaveAlias,
  handleDeleteAlias,
  handleSupersedeDevice,
}: DeviceManagementTabProps) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-6 mb-8">
      <section>
        <h3 className="font-display text-xl sm:text-2xl uppercase border-b-[3px] border-[#000000] pb-2 mb-4">{t('addDeviceAlias')}</h3>
        <div className="grid grid-cols-1 md:grid-cols-[1fr_1fr_auto] gap-2 border-[3px] border-[#000000] p-4 bg-[#F9F9F9]">
          <input
            type="text"
            value={newDeviceId}
            onChange={(e) => setNewDeviceId(e.target.value)}
            placeholder={t('deviceId')}
            className="border-[3px] border-[#000000] px-3 py-2 outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
          />
          <input
            type="text"
            value={newDeviceName}
            onChange={(e) => setNewDeviceName(e.target.value)}
            placeholder={t('displayName')}
            className="border-[3px] border-[#000000] px-3 py-2 outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
          />
          <button
            onClick={handleAddAlias}
            className="border-[3px] border-[#000000] px-5 py-2 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] transition-none whitespace-nowrap"
          >
            {t('addAlias')}
          </button>
        </div>
      </section>

      <section>
        <h3 className="font-display text-xl sm:text-2xl uppercase border-b-[3px] border-[#000000] pb-2 mb-4">{t('deviceManagement')}</h3>
        <div className="border-[3px] border-[#000000] overflow-x-auto">
          <table className="w-full text-left min-w-[960px]">
            <thead className="bg-[#000000] text-[#FFFFFF]">
              <tr>
                <th className="p-3 font-display uppercase">{t('deviceId')}</th>
                <th className="p-3 font-display uppercase">{t('displayName')}</th>
                <th className="p-3 font-display uppercase">{t('events')}</th>
                <th className="p-3 font-display uppercase">{t('totalTokens')}</th>
                <th className="p-3 font-display uppercase">{t('totalSpend')}</th>
                <th className="p-3 font-display uppercase">{t('firstSeen')}</th>
                <th className="p-3 font-display uppercase">{t('lastSeen')}</th>
                <th className="p-3 font-display uppercase text-center">{t('actions')}</th>
              </tr>
            </thead>
            <tbody>
              {devices.map((device) => {
                const totalTokens = device.input_tokens + device.cached_tokens + device.cache_creation_tokens + device.output_tokens;
                const hasAlias = device.display_name !== device.id;
                return (
                  <tr key={device.id} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#F0F0F0]">
                    <td className="p-3 font-mono text-[#666666] break-all">{device.id}</td>
                    <td className="p-3">
                      {editingDevice === device.id ? (
                        <div className="flex gap-2">
                          <input
                            type="text"
                            value={editAliasName}
                            onChange={(e) => setEditAliasName(e.target.value)}
                            className="min-w-[160px] border-[2px] border-[#000000] px-2 py-1 outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                          />
                          <button
                            onClick={() => handleSaveAlias(device.id)}
                            className="bg-[#008000] text-[#FFFFFF] border-[2px] border-[#008000] px-3 font-bold"
                          >
                            {t('save')}
                          </button>
                          <button
                            onClick={() => setEditingDevice(null)}
                            className="bg-[#FF0000] text-[#FFFFFF] border-[2px] border-[#FF0000] px-3 font-bold"
                          >
                            {t('cancel')}
                          </button>
                        </div>
                      ) : (
                        <span className="font-bold">{device.display_name}</span>
                      )}
                    </td>
                    <td className="p-3 font-mono">{device.events}</td>
                    <td className="p-3 font-mono">{formatTokens(totalTokens)}</td>
                    <td className="p-3 font-mono">${device.total_cost.toFixed(2)}</td>
                    <td className="p-3 font-mono text-xs">{formatDate(device.first_seen)}</td>
                    <td className="p-3 font-mono text-xs">{formatDate(device.last_seen)}</td>
                    <td className="p-3">
                      <div className="flex flex-wrap justify-center gap-2">
                        <button
                          onClick={() => {
                            setEditingDevice(device.id);
                            setEditAliasName(hasAlias ? device.display_name : '');
                          }}
                          className="border-[2px] border-[#000000] px-3 py-1 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF]"
                        >
                          {t('edit')}
                        </button>
                        <button
                          onClick={() => handleDeleteAlias(device.id)}
                          disabled={!hasAlias}
                          className="border-[2px] border-[#000000] px-3 py-1 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] disabled:opacity-40"
                        >
                          {t('clearAlias')}
                        </button>
                        <button
                          onClick={() => handleSupersedeDevice(device.id)}
                          className="border-[2px] border-[#FF0000] bg-[#FF0000] text-[#FFFFFF] px-3 py-1 font-bold uppercase hover:opacity-80"
                        >
                          {t('softDeleteDevice')}
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
              {devices.length === 0 && (
                <tr>
                  <td colSpan={8} className="p-3 text-center italic text-[#666666]">{t('noDevicesFound')}</td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </section>
    </div>
  );
}
