<?php

namespace FlowCatalyst\Generated\Exception;

class DeleteApiAdminConfigAccessByAppCodeByRoleCodeNotFoundException extends NotFoundException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodeDeleteResponse404
     */
    private $apiAdminConfigAccessAppCodeRoleCodeDeleteResponse404;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodeDeleteResponse404 $apiAdminConfigAccessAppCodeRoleCodeDeleteResponse404, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAccessAppCodeRoleCodeDeleteResponse404 = $apiAdminConfigAccessAppCodeRoleCodeDeleteResponse404;
        $this->response = $response;
    }
    public function getApiAdminConfigAccessAppCodeRoleCodeDeleteResponse404(): \FlowCatalyst\Generated\Model\ApiAdminConfigAccessAppCodeRoleCodeDeleteResponse404
    {
        return $this->apiAdminConfigAccessAppCodeRoleCodeDeleteResponse404;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}