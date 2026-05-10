import type { CSSProperties } from "react";

export function LazyBlockFallback() {
  return (
    <div className="mb-16 md:mb-20 border-[5px] border-[#000000] bg-[#FFFFFF] p-6 md:p-10">
      <div className="h-4 w-32 animate-pulse bg-[#000000]" aria-hidden="true"></div>
    </div>
  );
}

export function LazyModalFallback() {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[#000000]/80 p-4 md:p-6" style={{ "--wails-draggable": "no-drag" } as CSSProperties}>
      <div className="w-full max-w-5xl border-[5px] border-[#000000] bg-[#FFFFFF] p-6">
        <div className="h-5 w-40 animate-pulse bg-[#000000]" aria-hidden="true"></div>
      </div>
    </div>
  );
}
