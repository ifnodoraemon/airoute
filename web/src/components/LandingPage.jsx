import { useEffect, useRef, useState } from 'react';
import { getGatewayOrigin, copyToClipboard } from '../config';
import LandingNavbar from './landing/LandingNavbar';
import LandingHero from './landing/LandingHero';
import LandingSections from './landing/LandingSections';
import LandingCodeExamples from './landing/LandingCodeExamples';
import LandingFooter from './landing/LandingFooter';
import { getLandingSnippets } from './landing/landingSnippets';

function scrollToSection(id) {
  const el = document.getElementById(id);
  if (el) window.scrollTo({ top: el.getBoundingClientRect().top + window.pageYOffset - 70, behavior: 'smooth' });
}

export default function LandingPage({ isLoggedIn, onOpenLogin, onEnterConsole, onViewDocs, onViewStatus, lang = 'zh', toggleLang, theme, toggleTheme }) {
  const isZh = lang === 'zh';
  const [copiedSnippet, setCopiedSnippet] = useState('');
  const [activeTab, setActiveTab] = useState('python');
  const copyTimerRef = useRef(null);
  const copyRequestRef = useRef(0);
  const snippets = getLandingSnippets(getGatewayOrigin(), isZh);
  const entryProps = { isZh, isLoggedIn, onOpenLogin, onEnterConsole, onViewStatus };

  useEffect(() => {
    const hash = window.location.hash.replace(/^#\/?/, '').trim();
    if (!['features', 'multimodal', 'docs', 'pricing'].includes(hash)) return;
    const timerId = setTimeout(() => scrollToSection(hash), 150);
    return () => clearTimeout(timerId);
  }, []);

  useEffect(() => {
    const requestRef = copyRequestRef;
    return () => {
      ++requestRef.current;
      clearTimeout(copyTimerRef.current);
    };
  }, []);

  const copyCode = async (text, key) => {
    clearTimeout(copyTimerRef.current);
    const request = ++copyRequestRef.current;
    await copyToClipboard(text);
    if (request !== copyRequestRef.current) return;
    setCopiedSnippet(key);
    copyTimerRef.current = setTimeout(() => setCopiedSnippet(''), 2500);
  };

  return (
    <div className="min-h-screen bg-page text-primary flex flex-col font-sans selection:bg-brand selection:text-white">
      <LandingNavbar {...entryProps} {...{ scrollToSection, theme, toggleTheme, toggleLang }} />
      <LandingHero {...entryProps} {...{ onViewDocs, scrollToSection }} />
      <LandingSections isZh={isZh}>
        <LandingCodeExamples {...{ isZh, snippets, activeTab, setActiveTab, copiedSnippet, copyCode }} />
      </LandingSections>
      <LandingFooter {...entryProps} />
    </div>
  );
}
