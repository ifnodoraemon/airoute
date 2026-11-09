import { Layers, Percent, Coins } from 'lucide-react';

export default function PricingNavigation({
  isAdmin, isZh, localGroupMultipliers, prices, pricingSubTab, setPricingSubTab
}) {
  return (
    <div className="flex flex-wrap items-center gap-2 border-b border-[#E4E6E9] dark:border-[#2C3038] pb-2">
      <button
        type="button"
        onClick={() => setPricingSubTab('models')}
        className={`flex items-center space-x-2 px-3.5 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
          pricingSubTab === 'models'
            ? 'bg-[#355CFF] text-white shadow-xs'
            : 'bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
        }`}
      >
        <Layers className="w-4 h-4" />
        <span>{isZh ? '对外售价' : 'Customer prices'}</span>
        <span className={`text-[10px] px-1.5 py-0.2 rounded-full font-mono ${pricingSubTab === 'models' ? 'bg-white/20 text-white' : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]'}`}>
          {prices.length}
        </span>
      </button>

      <button
        type="button"
        onClick={() => setPricingSubTab('groups')}
        className={`flex items-center space-x-2 px-3.5 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
          pricingSubTab === 'groups'
            ? 'bg-[#355CFF] text-white shadow-xs'
            : 'bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
        }`}
      >
        <Percent className="w-4 h-4" />
        <span>{isZh ? '客户分组' : 'Group Discount Tiers'}</span>
        <span className={`text-[10px] px-1.5 py-0.2 rounded-full font-mono ${pricingSubTab === 'groups' ? 'bg-white/20 text-white' : 'bg-[#F0F1F2] dark:bg-[#22252B] text-[#656B75] dark:text-[#A1A7B0]'}`}>
          {Object.keys(localGroupMultipliers).length}
        </span>
      </button>

      {/* Supplier prices sit next to our selling prices: the two lists are
          stored separately, so this tab only pairs them and shows the spread. */}
      {isAdmin && <button
        type="button"
        onClick={() => setPricingSubTab('suppliers')}
        className={`flex items-center space-x-2 px-3.5 py-1.5 rounded-lg text-xs font-medium transition cursor-pointer ${
          pricingSubTab === 'suppliers'
            ? 'bg-[#355CFF] text-white shadow-xs'
            : 'bg-white dark:bg-[#191B1F] border border-[#E4E6E9] dark:border-[#2C3038] text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
        }`}
      >
        <Coins className="w-4 h-4" />
        <span>{isZh ? '渠道采购报价' : 'Supplier Prices'}</span>
      </button>}
    </div>
  );
}
