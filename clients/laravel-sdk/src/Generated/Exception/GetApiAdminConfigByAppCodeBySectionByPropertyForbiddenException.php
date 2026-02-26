<?php

namespace FlowCatalyst\Generated\Exception;

class GetApiAdminConfigByAppCodeBySectionByPropertyForbiddenException extends ForbiddenException
{
    /**
     * @var \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse403
     */
    private $apiAdminConfigAppCodeSectionPropertyGetResponse403;
    /**
     * @var \Psr\Http\Message\ResponseInterface
     */
    private $response;
    public function __construct(\FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse403 $apiAdminConfigAppCodeSectionPropertyGetResponse403, \Psr\Http\Message\ResponseInterface $response)
    {
        parent::__construct('Default Response');
        $this->apiAdminConfigAppCodeSectionPropertyGetResponse403 = $apiAdminConfigAppCodeSectionPropertyGetResponse403;
        $this->response = $response;
    }
    public function getApiAdminConfigAppCodeSectionPropertyGetResponse403(): \FlowCatalyst\Generated\Model\ApiAdminConfigAppCodeSectionPropertyGetResponse403
    {
        return $this->apiAdminConfigAppCodeSectionPropertyGetResponse403;
    }
    public function getResponse(): \Psr\Http\Message\ResponseInterface
    {
        return $this->response;
    }
}