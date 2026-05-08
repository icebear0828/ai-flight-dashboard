import React from "react";

type ErrorBoundaryState = {
  error: Error | null;
};

export default class ErrorBoundary extends React.Component<React.PropsWithChildren, ErrorBoundaryState> {
  state: ErrorBoundaryState = { error: null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidCatch(error: Error, info: React.ErrorInfo) {
    console.error("Dashboard render failed", error, info);
  }

  render() {
    if (this.state.error) {
      return (
        <div className="min-h-screen bg-[#FFFFFF] text-[#FF0000] p-4 sm:p-6 md:p-10 flex items-center justify-center font-display text-xl sm:text-2xl uppercase border-[12px] border-[#FF0000] m-3">
          <div className="text-center max-w-[900px]">
            <div className="text-4xl md:text-5xl mb-4 text-[#000000] bg-[#FF0000] inline-block px-6 py-2">
              SYSTEM ERROR
            </div>
            <div className="font-mono text-sm md:text-base normal-case break-words mb-6">
              {this.state.error.message || String(this.state.error)}
            </div>
            <button
              className="border-[3px] border-[#000000] bg-[#FFFFFF] text-[#000000] px-4 py-2 font-bold text-sm uppercase cursor-pointer"
              onClick={() => this.setState({ error: null })}
            >
              RESET VIEW
            </button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
