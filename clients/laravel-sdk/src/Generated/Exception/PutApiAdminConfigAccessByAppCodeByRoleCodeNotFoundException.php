<?php

namespace FlowCatalyst\Generated\Exception;

class PutApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse404
     */
    private $apiAdminConfigAccessAppCodeRoleCodePutResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse404 $apiAdminConfigAccessAppCodeRoleCodePutResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAccessAppCodeRoleCodePutResponse404 = $apiAdminConfigAccessAppCodeRoleCodePutResponse404;
        $this->response = $response;
    }
    public function getApiAdminConfigAccessAppCodeRoleCodePutResponse404(): \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodePutResponse404
    {
        return $this->apiAdminConfigAccessAppCodeRoleCodePutResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}