import { useState, useRef } from 'react';

export default function useCopilotState() {
  const [messages, setMessages] = useState([]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [copiedId, setCopiedId] = useState(null);
  const [isExpanded, setIsExpanded] = useState(false);
  const [selectedModel, setSelectedModel] = useState('');
  const [thinkingByModel,setThinkingByModel]=useState({});
  const [expandedReasoningIds, setExpandedReasoningIds] = useState({});
  const [expandedTraceIds, setExpandedTraceIds] = useState({});
  const [expandedStepObsIds, setExpandedStepObsIds] = useState({});
  const [showToolsModal, setShowToolsModal] = useState(false);
  const [activeBenchmark, setActiveBenchmark] = useState(null);

  const messagesEndRef = useRef(null);
  const inputRef = useRef(null);
  const abortControllerRef = useRef(null);
  return {
    abortControllerRef, activeBenchmark, copiedId, expandedReasoningIds, expandedStepObsIds, expandedTraceIds,
    input, inputRef, isExpanded, isLoading, messages, messagesEndRef, selectedModel, setActiveBenchmark,
    setCopiedId, setExpandedReasoningIds, setExpandedStepObsIds, setExpandedTraceIds, setInput, setIsExpanded,
    setIsLoading, setMessages, setSelectedModel, setShowToolsModal, setThinkingByModel, showToolsModal,
    thinkingByModel
  };
}
