/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useRef, useState } from 'react';
import { Button, Col, Form, Row, Spin } from '@douyinfe/semi-ui';
import { API, compareObjects, showError, showSuccess, showWarning } from '../../../helpers';
import { useTranslation } from 'react-i18next';

const KEY_ENABLED = 'global_channel_sticky_setting.enabled';
const KEY_TTL = 'global_channel_sticky_setting.ttl_seconds';

export default function SettingsGlobalChannelSticky(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    [KEY_ENABLED]: false,
    [KEY_TTL]: 3600,
  });
  const refForm = useRef();
  const [inputsRow, setInputsRow] = useState(inputs);

  function handleFieldChange(fieldName) {
    return (value) => {
      setInputs((prev) => ({ ...prev, [fieldName]: value }));
    };
  }

  function onSubmit() {
    const updateArray = compareObjects(inputs, inputsRow);
    if (!updateArray.length) return showWarning(t('你似乎并没有修改什么'));
    const requestQueue = updateArray.map((item) => {
      const value =
        typeof inputs[item.key] === 'boolean'
          ? String(inputs[item.key])
          : String(inputs[item.key]);
      return API.put('/api/option/', { key: item.key, value });
    });
    setLoading(true);
    Promise.all(requestQueue)
      .then((res) => {
        if (res.includes(undefined)) return showError(t('部分保存失败，请重试'));
        showSuccess(t('保存成功'));
        props.refresh();
      })
      .catch(() => showError(t('保存失败，请重试')))
      .finally(() => setLoading(false));
  }

  useEffect(() => {
    const currentInputs = {};
    for (const key in props.options) {
      if (Object.keys(inputs).includes(key)) {
        currentInputs[key] = props.options[key];
      }
    }
    setInputs((prev) => ({ ...prev, ...currentInputs }));
    setInputsRow((prev) => ({ ...prev, ...currentInputs }));
    if (refForm.current) {
      refForm.current.setValues({ ...inputs, ...currentInputs });
    }
  }, [props.options]);

  return (
    <Spin spinning={loading}>
      <Form
        values={inputs}
        getFormApi={(api) => (refForm.current = api)}
        style={{ marginBottom: 15 }}
      >
        <Form.Section text={t('全局渠道粘性')}>
          <Row gutter={16}>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.Switch
                field={KEY_ENABLED}
                label={t('启用全局渠道粘性')}
                extraText={t('启用后所有请求优先路由到同一渠道，遇到 429 或 401 时自动切换到下一个渠道')}
                size='default'
                checkedText='｜'
                uncheckedText='〇'
                onChange={handleFieldChange(KEY_ENABLED)}
              />
            </Col>
            <Col xs={24} sm={12} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field={KEY_TTL}
                label={t('活跃渠道缓存时长（秒）')}
                extraText={t('活跃渠道在 Redis 中的保留时间，0 表示永不过期，建议设置 3600')}
                step={60}
                min={0}
                placeholder='3600'
                onChange={handleFieldChange(KEY_TTL)}
              />
            </Col>
          </Row>
          <Row>
            <Button size='default' onClick={onSubmit}>
              {t('保存全局渠道粘性设置')}
            </Button>
          </Row>
        </Form.Section>
      </Form>
    </Spin>
  );
}
