import React from 'react';

type NavbarProps = {
  activeTab: string;
  onTabChange: (tab: 'gemini' | 'client') => void;
  onLogout: () => void;
};

const Navbar: React.FC<NavbarProps> = ({ activeTab, onTabChange, onLogout }) => {
  return (
    <div className="navbar bg-base-300 shadow-lg">
      <div className="navbar-start">
        <a className="btn btn-ghost text-xl">Gemini Balance</a>
      </div>
      <div className="navbar-center">
        <div className="tabs tabs-boxed">
          <a
            className={`tab ${activeTab === 'gemini' ? 'tab-active' : ''}`}
            onClick={() => onTabChange('gemini')}
          >
            Gemini Keys
          </a>
          <a
            className={`tab ${activeTab === 'client' ? 'tab-active' : ''}`}
            onClick={() => onTabChange('client')}
          >
            Client Keys
          </a>
        </div>
      </div>
      <div className="navbar-end">
        <button className="btn btn-outline btn-sm" onClick={onLogout}>Logout</button>
      </div>
    </div>
  );
};

export default Navbar;