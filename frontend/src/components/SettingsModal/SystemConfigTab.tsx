import React from 'react';
import { useTranslation } from 'react-i18next';
import { AppConfig, DeviceInfo } from '../../SettingsModal';

interface SystemConfigTabProps {
  config: AppConfig;
  devices: DeviceInfo[];
  newPath: string;
  setNewPath: (path: string) => void;
  handleAddPath: () => void;
  handleRemovePath: (index: number) => void;
  editingDevice: string | null;
  setEditingDevice: (id: string | null) => void;
  editAliasName: string;
  setEditAliasName: (name: string) => void;
  handleSaveAlias: (id: string) => void;
}

export default function SystemConfigTab({
  config,
  devices,
  newPath,
  setNewPath,
  handleAddPath,
  handleRemovePath,
  editingDevice,
  setEditingDevice,
  editAliasName,
  setEditAliasName,
  handleSaveAlias,
}: SystemConfigTabProps) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-col gap-8 mb-8">
      {/* Device Aliases Section */}
      <section>
        <h3 className="font-display text-xl sm:text-2xl uppercase border-b-[3px] border-[#000000] pb-2 mb-4">{t('deviceAliases')}</h3>
        <div className="border-[3px] border-[#000000] overflow-x-auto">
          <table className="w-full text-left min-w-[400px]">
            <thead className="bg-[#000000] text-[#FFFFFF]">
              <tr>
                <th className="p-3 font-display uppercase w-1/3">{t('deviceId')}</th>
                <th className="p-3 font-display uppercase w-2/3">{t('displayName')}</th>
              </tr>
            </thead>
            <tbody>
              {devices.map((d) => (
                <tr key={d.id} className="border-b-[3px] border-[#000000] last:border-b-0 hover:bg-[#F0F0F0]">
                  <td className="p-3 font-mono text-[#666666]">{d.id}</td>
                  <td className="p-3">
                    {editingDevice === d.id ? (
                      <div className="flex gap-2 flex-wrap">
                        <input 
                          type="text" 
                          value={editAliasName} 
                          onChange={e => setEditAliasName(e.target.value)}
                          className="flex-1 min-w-[120px] border-[2px] border-[#000000] px-2 py-1 outline-none focus:bg-[#000000] focus:text-[#FFFFFF]"
                        />
                        <button 
                          onClick={() => handleSaveAlias(d.id)}
                          className="bg-[#008000] text-[#FFFFFF] border-[2px] border-[#008000] px-3 font-bold hover:opacity-80 whitespace-nowrap"
                        >
                          {t('save')}
                        </button>
                        <button 
                          onClick={() => setEditingDevice(null)}
                          className="bg-[#FF0000] text-[#FFFFFF] border-[2px] border-[#FF0000] px-3 font-bold hover:opacity-80 whitespace-nowrap"
                        >
                          {t('cancel')}
                        </button>
                      </div>
                    ) : (
                      <div className="flex justify-between items-center group">
                        <span className="font-bold">{d.display_name}</span>
                        <button 
                          onClick={() => { setEditingDevice(d.id); setEditAliasName(d.display_name === d.id ? '' : d.display_name); }}
                          className="opacity-100 sm:opacity-0 group-hover:opacity-100 underline text-[#0000FF] shrink-0 ml-4"
                        >
                          {t('edit')}
                        </button>
                      </div>
                    )}
                  </td>
                </tr>
              ))}
              {devices.length === 0 && (
                <tr><td colSpan={2} className="p-3 text-center italic text-[#666666]">{t('noDevicesFound')}</td></tr>
              )}
            </tbody>
          </table>
        </div>
      </section>

      {/* Log Directories Section */}
      <section>
        <h3 className="font-display text-xl sm:text-2xl uppercase border-b-[3px] border-[#000000] pb-2 mb-4">{t('extraWatchDirectories')}</h3>
        <p className="mb-4 text-[#666666]">{t('extraWatchDesc')}</p>
        
        <ul className="mb-4 flex flex-col gap-2">
          {(config.extra_watch_dirs || []).map((dir, idx) => (
            <li key={idx} className="flex flex-col sm:flex-row sm:justify-between items-start sm:items-center gap-2 sm:gap-4 border-[3px] border-[#000000] bg-[#F9F9F9] p-3">
              <span className="font-mono break-all sm:truncate w-full">{dir}</span>
              <button 
                onClick={() => handleRemovePath(idx)}
                className="bg-[#FF0000] text-[#FFFFFF] font-bold px-3 py-1 border-[2px] border-[#FF0000] hover:opacity-80 shrink-0"
              >
                {t('remove')}
              </button>
            </li>
          ))}
          {(config.extra_watch_dirs || []).length === 0 && (
            <li className="italic text-[#666666] border-[3px] border-dashed border-[#CCCCCC] p-3 text-center">{t('noExtraDirectories')}</li>
          )}
        </ul>

        <div className="flex flex-col sm:flex-row gap-2">
          <input 
            type="text" 
            value={newPath}
            onChange={(e) => setNewPath(e.target.value)}
            placeholder={t('addPathPlaceholder')}
            className="flex-1 border-[3px] border-[#000000] px-3 py-2 outline-none focus:bg-[#000000] focus:text-[#FFFFFF] transition-none w-full"
          />
          <button 
            onClick={handleAddPath}
            className="border-[3px] border-[#000000] px-6 py-2 font-bold uppercase hover:bg-[#000000] hover:text-[#FFFFFF] transition-none whitespace-nowrap w-full sm:w-auto"
          >
            {t('addPath')}
          </button>
        </div>
      </section>
    </div>
  );
}
