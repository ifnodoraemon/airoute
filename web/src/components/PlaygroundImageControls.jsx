import { Sparkles, RefreshCw } from 'lucide-react';

export default function PlaygroundImageControls({
  handleGenerateImage, imgModel, imgModels, imgPrompt, imgSize, isZh, playLoading, setImgModel, setImgPrompt,
  setImgSize
}) {
  return (
    <div className="space-y-3">
      <div>
        <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '生图模型' : 'Image Model'}</label>
        <select
          value={imgModel}
          onChange={(e) => setImgModel(e.target.value)}
          className="ui-field font-mono"
        >
          {imgModels.map(m => (
            <option key={m} value={m}>{m}</option>
          ))}
        </select>
      </div>

      <div>
        <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '画面尺寸 (Size)' : 'Image Size'}</label>
        <select
          value={imgSize}
          onChange={(e) => setImgSize(e.target.value)}
          className="ui-field"
        >
          <option value="1024x1024">{isZh ? '1024x1024 (方形 1:1)' : '1024x1024 (Square 1:1)'}</option>
          <option value="1792x1024">{isZh ? '1792x1024 (横屏 16:9)' : '1792x1024 (Landscape 16:9)'}</option>
          <option value="1024x1792">{isZh ? '1024x1792 (竖屏 9:16)' : '1024x1792 (Portrait 9:16)'}</option>
        </select>
      </div>

      <div>
        <label className="block text-xs font-semibold text-slate-600 dark:text-slate-400 mb-1">{isZh ? '画面提示词 (Prompt)' : 'Prompt'}</label>
        <textarea
          rows={3}
          value={imgPrompt}
          onChange={(e) => setImgPrompt(e.target.value)}
          className="ui-field resize-y"
        />
      </div>

      <button
        onClick={handleGenerateImage}
        disabled={playLoading}
        className="ui-btn w-full bg-pink-600 text-white hover:bg-pink-700 shadow-sm"
      >
        {playLoading ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Sparkles className="w-3.5 h-3.5" />}
        <span>{playLoading ? (isZh ? '渲染生成中...' : 'Rendering...') : (isZh ? '生成图像' : 'Generate Image')}</span>
      </button>
    </div>
  );
}
