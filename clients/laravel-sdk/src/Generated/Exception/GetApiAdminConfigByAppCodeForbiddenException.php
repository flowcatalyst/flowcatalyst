<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminConfigByAppCodeForbiddenException extends ForbiddenException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeGetResponse403
     */
    private $apiAdminConfigAppCodeGetResponse403;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeGetResponse403 $apiAdminConfigAppCodeGetResponse403, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAppCodeGetResponse403 = $apiAdminConfigAppCodeGetResponse403;
        $this->response = $response;
    }
    public function getApiAdminConfigAppCodeGetResponse403(): \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeGetResponse403
    {
        return $this->apiAdminConfigAppCodeGetResponse403;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}