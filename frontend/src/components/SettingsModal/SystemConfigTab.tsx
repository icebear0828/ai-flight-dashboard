import React from 'react';
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
  return (
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
  );
}
