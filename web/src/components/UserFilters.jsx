import { RefreshCw, Search } from 'lucide-react';

export default function UserFilters({
  fetchUsers, isZh, loading, roleFilter, searchFilter, setRoleFilter, setSearchFilter
}) {
  return (
    <div className="ui-card p-3 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
      <div className="flex items-center space-x-3 flex-1 max-w-md">
        <div className="relative w-full">
          <Search className="w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] absolute left-3 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={searchFilter}
            onChange={(e) => setSearchFilter(e.target.value)}
            placeholder={isZh ? '搜索用户名、邮箱、分组名称...' : 'Search username, email, group...'}
            className="ui-field ui-field-sm pl-9 w-full"
          />
        </div>
      </div>

      <div className="flex items-center space-x-2">
        <div className="flex items-center space-x-0.5 bg-[#F0F1F2] dark:bg-[#22252B] p-0.5 rounded-lg border border-[#E4E6E9] dark:border-[#2C3038] text-xs font-medium">
          <button
            onClick={() => setRoleFilter('all')}
            className={`px-3 py-1 rounded-md transition cursor-pointer ${
              roleFilter === 'all'
                ? 'bg-white dark:bg-[#191B1F] text-[#355CFF] dark:text-[#91A7FF] shadow-xs font-semibold'
                : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
            }`}
          >
            {isZh ? '全部' : 'All'}
          </button>
          <button
            onClick={() => setRoleFilter('admin')}
            className={`px-3 py-1 rounded-md transition cursor-pointer ${
              roleFilter === 'admin'
                ? 'bg-white dark:bg-[#191B1F] text-purple-600 dark:text-purple-400 shadow-xs font-semibold'
                : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
            }`}
          >
            {isZh ? '管理员' : 'Admins'}
          </button>
          <button
            onClick={() => setRoleFilter('user')}
            className={`px-3 py-1 rounded-md transition cursor-pointer ${
              roleFilter === 'user'
                ? 'bg-white dark:bg-[#191B1F] text-emerald-600 dark:text-emerald-400 shadow-xs font-semibold'
                : 'text-[#656B75] dark:text-[#A1A7B0] hover:text-[#202124] dark:hover:text-[#F1F2F4]'
            }`}
          >
            {isZh ? '普通用户' : 'Users'}
          </button>
        </div>

        <button
          onClick={fetchUsers}
          disabled={loading}
          className="ui-btn-secondary ui-btn-sm"
          title={isZh ? '刷新列表' : 'Refresh list'}
        >
          <RefreshCw className={`w-3.5 h-3.5 text-[#656B75] dark:text-[#A1A7B0] ${loading ? 'animate-spin' : ''}`} />
        </button>
      </div>
    </div>
  );
}
