import { useState } from 'react';
import useModalA11y from './useModalA11y';
import { getGatewayOrigin } from '../config';

export default function useSkillState() {
  const [skills, setSkills] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [togglingSkill, setTogglingSkill] = useState({});
  const [storageStatus, setStorageStatus] = useState(null);

  // Modals
  const [manifestSkill, setManifestSkill] = useState(null);
  const [manifestTab, setManifestTab] = useState('skill_md'); // 'skill_md' | 'bundle_tree' | 'metadata'
  const [clientGuideModalOpen, setClientGuideModalOpen] = useState(false);
  const [selectedSkillForGuide, setSelectedSkillForGuide] = useState(null);
  const [activeClientTab, setActiveClientTab] = useState('claude-code'); // 'claude-code' | 'opencode' | 'codex' | 'cursor'

  const [testModalSkill, setTestModalSkill] = useState(null);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const [editingSkillId, setEditingSkillId] = useState('');
  const [savingSkill, setSavingSkill] = useState(false);

  useModalA11y(clientGuideModalOpen, () => setClientGuideModalOpen(false));
  useModalA11y(!!manifestSkill, () => setManifestSkill(null));
  useModalA11y(createModalOpen, () => setCreateModalOpen(false));

  // New Skill Form
  const [newSkill, setNewSkill] = useState({
    id: '',
    name: '',
    category: 'dev',
    description: '',
    author: 'Community Standard',
    version: '1.0.0',
    loading_mode: 'lazy',
    tools: '',
    manifest: ''
  });

  const [copiedKey, setCopiedKey] = useState('');
  const origin = getGatewayOrigin();
  return {
    activeClientTab, clientGuideModalOpen, copiedKey, createModalOpen, editingSkillId, loading, manifestSkill,
    manifestTab, newSkill, origin, savingSkill, searchQuery, selectedCategory, selectedSkillForGuide,
    setActiveClientTab, setClientGuideModalOpen, setCopiedKey, setCreateModalOpen, setEditingSkillId,
    setLoading, setManifestSkill, setManifestTab, setNewSkill, setSavingSkill, setSearchQuery,
    setSelectedCategory, setSelectedSkillForGuide, setSkills, setStorageStatus, setTestModalSkill,
    setTogglingSkill, skills, storageStatus, testModalSkill, togglingSkill
  };
}
