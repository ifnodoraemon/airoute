import { useState, useEffect, useRef } from 'react';

export default function useCopilotDrag({
  isExpanded, onToggle
}) {
  const [btnPos, setBtnPos] = useState(() => {
    try {
      const saved = localStorage.getItem('airoute_copilot_btn_pos');
      if (saved) {
        const parsed = JSON.parse(saved);
        if (typeof parsed.x === 'number' && typeof parsed.y === 'number') {
          return parsed;
        }
      }
    } catch (e) {}
    return null;
  });

  const [isBtnDragging, setIsBtnDragging] = useState(false);
  const btnDragRef = useRef({
    startX: 0,
    startY: 0,
    initialX: 0,
    initialY: 0,
    hasMoved: false
  });
  const justDraggedBtnRef = useRef(false);
  const btnRef = useRef(null);

  // Modal window draggable state (when not expanded)
  const [modalPos, setModalPos] = useState(null);
  const modalDragRef = useRef({
    startX: 0,
    startY: 0,
    initialX: 0,
    initialY: 0,
    hasMoved: false
  });
  const modalRef = useRef(null);

  // Initialize and ensure safe button position on window resize
  useEffect(() => {
    const computeSafeDefault = () => {
      const w = typeof window !== 'undefined' ? window.innerWidth : 1200;
      const h = typeof window !== 'undefined' ? window.innerHeight : 800;
      // Default: 96px from bottom (safely clears table pagination bar), 24px from right
      return {
        x: Math.max(16, w - 190),
        y: Math.max(16, h - 96)
      };
    };

    const safePosition = prev => {
      if (!prev) return computeSafeDefault();
      const w = window.innerWidth;
      const h = window.innerHeight;
      return {
        x: Math.min(Math.max(16, prev.x), Math.max(16, w - 180)),
        y: Math.min(Math.max(16, prev.y), Math.max(16, h - 56))
      };
    };
    const handleResize = () => setBtnPos(safePosition);
    handleResize();

    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  const handleBtnPointerDown = (e) => {
    if (e.button !== 0) return;
    e.preventDefault();

    const currentX = btnPos?.x ?? (window.innerWidth - 190);
    const currentY = btnPos?.y ?? (window.innerHeight - 96);

    btnDragRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      initialX: currentX,
      initialY: currentY,
      hasMoved: false
    };

    const handlePointerMove = (moveEvent) => {
      const dx = moveEvent.clientX - btnDragRef.current.startX;
      const dy = moveEvent.clientY - btnDragRef.current.startY;

      if (!btnDragRef.current.hasMoved && Math.hypot(dx, dy) > 4) {
        btnDragRef.current.hasMoved = true;
        setIsBtnDragging(true);
      }

      if (btnDragRef.current.hasMoved) {
        const btnW = btnRef.current ? btnRef.current.offsetWidth : 160;
        const btnH = btnRef.current ? btnRef.current.offsetHeight : 44;
        const maxX = Math.max(12, window.innerWidth - btnW - 12);
        const maxY = Math.max(12, window.innerHeight - btnH - 12);

        const newX = Math.min(maxX, Math.max(12, btnDragRef.current.initialX + dx));
        const newY = Math.min(maxY, Math.max(12, btnDragRef.current.initialY + dy));

        setBtnPos({ x: newX, y: newY });
      }
    };

    const handlePointerUp = () => {
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', handlePointerUp);
      window.removeEventListener('pointercancel', handlePointerUp);

      if (btnDragRef.current.hasMoved) {
        setIsBtnDragging(false);
        justDraggedBtnRef.current = true;
        setTimeout(() => {
          justDraggedBtnRef.current = false;
        }, 120);

        setBtnPos(currentPos => {
          if (currentPos) {
            try {
              localStorage.setItem('airoute_copilot_btn_pos', JSON.stringify(currentPos));
            } catch (err) {}
          }
          return currentPos;
        });
      }
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', handlePointerUp);
    window.addEventListener('pointercancel', handlePointerUp);
  };

  const handleBtnClick = (e) => {
    if (justDraggedBtnRef.current || btnDragRef.current.hasMoved) {
      e.preventDefault();
      e.stopPropagation();
      return;
    }
    onToggle();
  };

  const handleModalHeaderPointerDown = (e) => {
    if (isExpanded) return;
    if (e.target.closest('button, select, input')) return;
    if (e.button !== 0) return;
    e.preventDefault();

    const rect = modalRef.current?.getBoundingClientRect();
    const currentX = rect ? rect.left : (window.innerWidth - 440);
    const currentY = rect ? rect.top : (window.innerHeight - 660);

    modalDragRef.current = {
      startX: e.clientX,
      startY: e.clientY,
      initialX: currentX,
      initialY: currentY,
      hasMoved: false
    };

    const handlePointerMove = (moveEvent) => {
      const dx = moveEvent.clientX - modalDragRef.current.startX;
      const dy = moveEvent.clientY - modalDragRef.current.startY;

      if (!modalDragRef.current.hasMoved && Math.hypot(dx, dy) > 4) {
        modalDragRef.current.hasMoved = true;
      }

      if (modalDragRef.current.hasMoved) {
        const modalW = modalRef.current ? modalRef.current.offsetWidth : 420;
        const modalH = modalRef.current ? modalRef.current.offsetHeight : 640;
        const maxX = Math.max(12, window.innerWidth - modalW - 12);
        const maxY = Math.max(12, window.innerHeight - modalH - 12);

        const newX = Math.min(maxX, Math.max(12, modalDragRef.current.initialX + dx));
        const newY = Math.min(maxY, Math.max(12, modalDragRef.current.initialY + dy));

        setModalPos({ x: newX, y: newY });
      }
    };

    const handlePointerUp = () => {
      window.removeEventListener('pointermove', handlePointerMove);
      window.removeEventListener('pointerup', handlePointerUp);
      window.removeEventListener('pointercancel', handlePointerUp);
    };

    window.addEventListener('pointermove', handlePointerMove);
    window.addEventListener('pointerup', handlePointerUp);
    window.addEventListener('pointercancel', handlePointerUp);
  };
  return {
    btnPos, btnRef, handleBtnClick, handleBtnPointerDown, handleModalHeaderPointerDown, isBtnDragging, modalPos,
    modalRef
  };
}
