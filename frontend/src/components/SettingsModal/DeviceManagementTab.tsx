import React, { useState } from 'react';
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
  deletingDevice: string | null;
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
  deletingDevice,
  handleSaveAlias,
  handleDeleteAlias,
  handleSupersedeDevice,
}: DeviceManagementTabProps) {
  const { t } = useTranslation();
  const [confirmingDevice, setConfirmingDevice] = useState<string | null>(null);

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
        <div data-testid="device-management-list" className="flex flex-col gap-3">
          {devices.map((device) => {
            const totalTokens = device.input_tokens + device.cached_tokens + device.cache_creation_tokens + device.output_tokens;
            const hasAlias = device.display_name !== device.id;
            const isConfirming = confirmingDevice === device.id;
            const isDeleting = deletingDevice === device.id;

            return (
              <article
                key={device.id}
                data-testid="device-row"
                className="border-[3px] border-[#000000] bg-[#FFFFFF] p-3 sm:p-4"
              >
                <div className="grid grid-cols-1 gap-3 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-start">
                  <div className="min-w-0">
                    <div className="font-display uppercase text-sm text-[#666666]">{t('deviceId')}</div>
                    <div className="font-mono break-all text-sm sm:text-base">{device.id}</div>
                    <div className="mt-3 font-display uppercase text-sm text-[#666666]">{t('displayName')}</div>
                    {editingDevice === device.id ? (
                      <div className="mt-1 flex flex-col gap-2 sm:flex-row">
                        <input
                          type="text"
                          value={editAliasName}
                          onChange={(e) => setEditAliasName(e.target.value)}
                          className="min-w-0 flex-1 border-[2px] border-[#000000] px-2 py-2 outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                        />
                        <button
                          onClick={() => handleSaveAlias(device.id)}
                          className="border-[2px] border-[#008000] bg-[#008000] px-3 py-2 font-bold uppercase text-[#FFFFFF]"
                        >
                          {t('save')}
                        </button>
                        <button
                          onClick={() => setEditingDevice(null)}
                          className="border-[2px] border-[#000000] px-3 py-2 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF]"
                        >
                          {t('cancel')}
                        </button>
                      </div>
                    ) : (
                      <div className="mt-1 break-all font-bold">{device.display_name}</div>
                    )}
                  </div>

                  <div className="flex flex-wrap gap-2 lg:max-w-[300px] lg:justify-end">
                    <button
                      onClick={() => {
                        setConfirmingDevice(null);
                        setEditingDevice(device.id);
                        setEditAliasName(hasAlias ? device.display_name : '');
                      }}
                      className="border-[2px] border-[#000000] px-3 py-2 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF]"
                    >
                      {t('edit')}
                    </button>
                    <button
                      onClick={() => handleDeleteAlias(device.id)}
                      disabled={!hasAlias}
                      className="border-[2px] border-[#000000] px-3 py-2 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] disabled:opacity-40"
                    >
                      {t('clearAlias')}
                    </button>
                    {isConfirming ? (
                      <>
                        <button
                          onClick={() => handleSupersedeDevice(device.id)}
                          disabled={isDeleting}
                          className="border-[2px] border-[#FF0000] bg-[#FF0000] px-3 py-2 font-bold uppercase text-[#FFFFFF] disabled:opacity-50"
                        >
                          {isDeleting ? t('deletingDevice') : t('confirmSoftDeleteAction')}
                        </button>
                        <button
                          onClick={() => setConfirmingDevice(null)}
                          disabled={isDeleting}
                          className="border-[2px] border-[#000000] px-3 py-2 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] disabled:opacity-50"
                        >
                          {t('cancel')}
                        </button>
                      </>
                    ) : (
                      <button
                        onClick={() => {
                          setEditingDevice(null);
                          setConfirmingDevice(device.id);
                        }}
                        className="border-[2px] border-[#FF0000] bg-[#FF0000] px-3 py-2 font-bold uppercase text-[#FFFFFF] hover:opacity-80"
                      >
                        {t('softDeleteDevice')}
                      </button>
                    )}
                  </div>
                </div>

                {isConfirming && (
                  <div className="mt-3 border-t-[2px] border-[#FF0000] pt-3 font-bold text-[#FF0000]" role="status">
                    {t('confirmSoftDeleteDevice', { id: device.id })}
                  </div>
                )}

                <dl className="mt-4 grid grid-cols-2 gap-x-4 gap-y-3 sm:grid-cols-3 lg:grid-cols-5">
                  <div>
                    <dt className="font-display uppercase text-xs text-[#666666]">{t('events')}</dt>
                    <dd className="font-mono">{device.events}</dd>
                  </div>
                  <div>
                    <dt className="font-display uppercase text-xs text-[#666666]">{t('totalTokens')}</dt>
                    <dd className="font-mono">{formatTokens(totalTokens)}</dd>
                  </div>
                  <div>
                    <dt className="font-display uppercase text-xs text-[#666666]">{t('totalSpend')}</dt>
                    <dd className="font-mono">${device.total_cost.toFixed(2)}</dd>
                  </div>
                  <div>
                    <dt className="font-display uppercase text-xs text-[#666666]">{t('firstSeen')}</dt>
                    <dd className="font-mono text-xs">{formatDate(device.first_seen)}</dd>
                  </div>
                  <div>
                    <dt className="font-display uppercase text-xs text-[#666666]">{t('lastSeen')}</dt>
                    <dd className="font-mono text-xs">{formatDate(device.last_seen)}</dd>
                  </div>
                </dl>
              </article>
            );
          })}
          {devices.length === 0 && (
            <div className="border-[3px] border-[#000000] p-4 text-center italic text-[#666666]">{t('noDevicesFound')}</div>
          )}
        </div>
      </section>
    </div>
  );
}
