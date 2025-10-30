-- 选择模块
UPDATE config_records SET value = '"DoubaoASR"', updated_at = datetime('now', 'localtime') WHERE key = 'Selected.ASR';
UPDATE config_records SET value = '"DoubaoLLM"', updated_at = datetime('now', 'localtime') WHERE key = 'Selected.LLM';
UPDATE config_records SET value = '"DoubaoTTS"', updated_at = datetime('now', 'localtime') WHERE key = 'Selected.TTS';
UPDATE config_records SET value = '"ChatGLMVLLM"', updated_at = datetime('now', 'localtime') WHERE key = 'Selected.VLLLM';

UPDATE model_selections
SET 
    asr_provider = 'DoubaoASR',
    tts_provider = 'DoubaoTTS',
    llm_provider = 'DoubaoLLM',
    vllm_provider = 'ChatGLMVLLM',
    updated_at = datetime('now', 'localtime')
WHERE id = 1;


-- Web 配置
UPDATE config_records SET value = '"http://127.0.0.1:8080/api/vision"', updated_at = datetime('now', 'localtime') WHERE key = 'Web.VisionURL';
UPDATE config_records SET value = '"ws://127.0.0.1:8000/ws"', updated_at = datetime('now', 'localtime') WHERE key = 'Web.Websocket';

-- ASR 配置
UPDATE config_records SET value = '"api"', updated_at = datetime('now', 'localtime') WHERE key = 'ASR.DoubaoASR.access_token';
UPDATE config_records SET value = '"id"', updated_at = datetime('now', 'localtime') WHERE key = 'ASR.DoubaoASR.appid';

-- LLM 配置
UPDATE config_records SET value = '"api"', updated_at = datetime('now', 'localtime') WHERE key = 'LLM.DoubaoLLM.APIKey';
UPDATE config_records SET value = '"https://ark.cn-beijing.volces.com/api/v3"', updated_at = datetime('now', 'localtime') WHERE key = 'LLM.DoubaoLLM.BaseURL';
UPDATE config_records SET value = '"deepseek-v3-1-terminus"', updated_at = datetime('now', 'localtime') WHERE key = 'LLM.DoubaoLLM.ModelName';

-- TTS 配置
UPDATE config_records SET value = '"id"', updated_at = datetime('now', 'localtime') WHERE key = 'TTS.DoubaoTTS.AppID';
UPDATE config_records SET value = '"api"', updated_at = datetime('now', 'localtime') WHERE key = 'TTS.DoubaoTTS.Token';
UPDATE config_records SET value = '"BV001_streaming"', updated_at = datetime('now', 'localtime') WHERE key = 'TTS.DoubaoTTS.Voice';
